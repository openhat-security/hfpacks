package proxy

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultProxyListURL matches the ProxyScrape feed used by the GitHub enumerator script.
	DefaultProxyListURL = "https://api.proxyscrape.com/v4/free-proxy-list/get?request=display_proxies&proxy_format=ipport&format=text&country=us"
	defaultProxyBatch   = 10
	defaultSwitchEvery  = 60
)

// PoolOpts configures an auto-fetched rotating proxy pool.
type PoolOpts struct {
	NoProxy     bool
	SwitchEvery int           // advance to next proxy every N successful Hub calls
	BatchSize   int           // how many proxies to keep from each Fetch
	Timeout     time.Duration // Hub request timeout
	ListURL     string        // override PROXY_LIST_URL / default scrape feed
}

// Pool fetches a batch of proxies, cycles with Next(), and refetches when empty.
type Pool struct {
	mu          sync.Mutex
	noProxy     bool
	switchEvery int
	batchSize   int
	timeout     time.Duration
	listURL     string
	authUser    string
	authPass    string

	proxies []string // "host:port" or full URLs
	idx     int
	calls   int
	client  *http.Client
	current string
}

// NewPool fetches the first proxy batch unless NoProxy is set.
func NewPool(opts PoolOpts) (*Pool, error) {
	if opts.SwitchEvery <= 0 {
		opts.SwitchEvery = defaultSwitchEvery
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultProxyBatch
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 45 * time.Second
	}
	listURL := strings.TrimSpace(opts.ListURL)
	if listURL == "" {
		listURL = strings.TrimSpace(os.Getenv("PROXY_LIST_URL"))
	}
	if listURL == "" {
		listURL = DefaultProxyListURL
	}

	p := &Pool{
		noProxy:     opts.NoProxy,
		switchEvery: opts.SwitchEvery,
		batchSize:   opts.BatchSize,
		timeout:     opts.Timeout,
		listURL:     listURL,
		authUser:    strings.TrimSpace(os.Getenv("GEONODE_USER")),
		authPass:    strings.TrimSpace(os.Getenv("GEONODE_PASS")),
	}

	// Optional sticky Geonode gateway: synthesize session proxies so we still rotate.
	if gw := strings.TrimSpace(os.Getenv("GEONODE_PROXY_URL")); gw != "" && !opts.NoProxy {
		if sessions := geonodeSessionBatch(gw, opts.BatchSize); len(sessions) > 0 {
			p.proxies = sessions
			p.idx = 0
			fmt.Fprintf(os.Stderr, "[+] loaded %d Geonode session proxies from GEONODE_PROXY_URL\n", len(p.proxies))
			if err := p.advanceLocked("init"); err != nil {
				return nil, err
			}
			return p, nil
		}
	}

	if p.noProxy {
		p.client = buildClient(nil, p.timeout)
		fmt.Fprintf(os.Stderr, "[+] proxy: direct (no-proxy)\n")
		return p, nil
	}

	if err := p.fetchLocked(); err != nil {
		return nil, err
	}
	if err := p.advanceLocked("init"); err != nil {
		return nil, err
	}
	return p, nil
}

// Client returns the current HTTP client.
func (p *Pool) Client() *http.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client
}

// MarkCall records a successful Hub call; advances proxy every SwitchEvery calls.
func (p *Pool) MarkCall() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.noProxy || p.switchEvery <= 0 {
		return
	}
	if p.calls%p.switchEvery == 0 {
		fmt.Fprintf(os.Stderr, "[*] rotating proxy (call #%d)\n", p.calls)
		_ = p.advanceLocked(fmt.Sprintf("call #%d", p.calls))
	}
}

// Rotate moves to the next proxy immediately (after 429 / transport errors).
func (p *Pool) Rotate(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.noProxy {
		return
	}
	fmt.Fprintf(os.Stderr, "[*] rotating proxy (%s)\n", reason)
	_ = p.advanceLocked(reason)
}

// Calls returns successful Hub calls counted so far.
func (p *Pool) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// Current returns the current proxy address (host:port or redacted URL).
func (p *Pool) Current() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

func (p *Pool) advanceLocked(reason string) error {
	addr, err := p.nextLocked()
	if err != nil {
		return err
	}
	u, err := parseProxyAddr(addr, p.authUser, p.authPass)
	if err != nil {
		return err
	}
	p.current = redactProxy(u)
	p.client = buildClient(u, p.timeout)
	if reason == "init" {
		fmt.Fprintf(os.Stderr, "[+] using proxy %s (switch every %d calls)\n", p.current, p.switchEvery)
	}
	return nil
}

func (p *Pool) nextLocked() (string, error) {
	if p.idx >= len(p.proxies) {
		if err := p.fetchLocked(); err != nil {
			return "", err
		}
	}
	if len(p.proxies) == 0 {
		return "", fmt.Errorf("no proxies available")
	}
	addr := p.proxies[p.idx]
	p.idx++
	return addr, nil
}

func (p *Pool) fetchLocked() error {
	fmt.Fprintf(os.Stderr, "[*] fetching proxy list…\n")
	raw, err := downloadProxyList(p.listURL)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	var valid []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		valid = append(valid, l)
	}
	if len(valid) == 0 {
		return fmt.Errorf("no proxies returned from %s", p.listURL)
	}
	rand.Shuffle(len(valid), func(i, j int) { valid[i], valid[j] = valid[j], valid[i] })
	if len(valid) > p.batchSize {
		valid = valid[:p.batchSize]
	}
	p.proxies = valid
	p.idx = 0
	fmt.Fprintf(os.Stderr, "[+] loaded %d proxies\n", len(p.proxies))
	return nil
}

func downloadProxyList(listURL string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Get(listURL)
	if err != nil {
		return "", fmt.Errorf("fetch proxies: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 400))
		return "", fmt.Errorf("fetch proxies: HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func buildClient(proxyURL *url.URL, timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != nil {
		tr.Proxy = http.ProxyURL(proxyURL)
	} else {
		tr.Proxy = nil
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

// parseProxyAddr accepts "ip:port", "http://ip:port", or a full URL.
// Optional Geonode user/pass are applied to bare ip:port entries.
func parseProxyAddr(addr, user, pass string) (*url.URL, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("empty proxy addr")
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid proxy addr %q", addr)
	}
	if u.User == nil && user != "" {
		if pass != "" {
			u.User = url.UserPassword(user, pass)
		} else {
			u.User = url.User(user)
		}
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	return u, nil
}

// geonodeSessionBatch builds N session URLs from a Geonode gateway URL so
// each "proxy" gets a distinct sticky session (common Geonode rotating pattern).
func geonodeSessionBatch(raw string, n int) []string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	if n <= 0 {
		n = defaultProxyBatch
	}
	baseUser := ""
	basePass := ""
	if u.User != nil {
		baseUser = u.User.Username()
		basePass, _ = u.User.Password()
	}
	if baseUser == "" {
		baseUser = strings.TrimSpace(os.Getenv("GEONODE_USER"))
		basePass = strings.TrimSpace(os.Getenv("GEONODE_PASS"))
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		cu := *u
		if baseUser != "" {
			sessionUser := fmt.Sprintf("%s-session-%d-%d", baseUser, time.Now().UnixNano()%1e6, i)
			if basePass != "" {
				cu.User = url.UserPassword(sessionUser, basePass)
			} else {
				cu.User = url.User(sessionUser)
			}
		}
		out = append(out, cu.String())
	}
	return out
}

func redactProxy(u *url.URL) string {
	if u == nil {
		return "none"
	}
	c := *u
	if c.User != nil {
		if _, has := c.User.Password(); has {
			c.User = url.UserPassword(c.User.Username(), "***")
		}
	}
	return c.String()
}
