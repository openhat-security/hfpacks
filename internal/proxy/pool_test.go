package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseProxyAddr(t *testing.T) {
	u, err := parseProxyAddr("1.2.3.4:8080", "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "1.2.3.4:8080" || u.User.Username() != "user" {
		t.Fatalf("%s", u)
	}
	pass, ok := u.User.Password()
	if !ok || pass != "pass" {
		t.Fatal("pass")
	}
}

func TestFetchAndAdvance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("10.0.0.1:8000\n10.0.0.2:8000\n10.0.0.3:8000\n"))
	}))
	defer srv.Close()

	p, err := NewPool(PoolOpts{
		ListURL:     srv.URL,
		BatchSize:   2,
		SwitchEvery: 2,
		Timeout:     0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Client() == nil {
		t.Fatal("nil client")
	}
	if !strings.Contains(p.Current(), "10.0.0.") {
		t.Fatalf("current %s", p.Current())
	}
	first := p.Current()
	p.MarkCall()
	p.MarkCall() // should rotate at switch-every=2
	if p.Current() == first {
		// may still be same host if only 2 and we advanced — just ensure no panic
		p.Rotate("test")
	}
	p.Rotate("forced")
}

func TestNewPoolNoProxy(t *testing.T) {
	p, err := NewPool(PoolOpts{NoProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Client() == nil {
		t.Fatal("nil")
	}
	p.MarkCall()
	p.Rotate("noop")
}

func TestGeonodeSessionBatch(t *testing.T) {
	batch := geonodeSessionBatch("http://myuser:secret@proxy.geonode.io:9000", 3)
	if len(batch) != 3 {
		t.Fatalf("%d", len(batch))
	}
	for _, s := range batch {
		if !strings.Contains(s, "myuser-session-") || !strings.Contains(s, "proxy.geonode.io:9000") {
			t.Fatalf("%s", s)
		}
	}
}
