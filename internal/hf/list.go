package hf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrRetryable indicates the caller should rotate proxy and retry once.
var ErrRetryable = errors.New("huggingface: retryable")

// ListOpts controls paginated Hub model listing.
type ListOpts struct {
	Query         string
	Author        string
	Task          string
	Library       string
	Filter        string
	Sort          string
	Direction     string
	Limit         int
	PageSize      int
	MaxPages      int
	Full          bool
	Sleep         time.Duration
	SinceUnix     int64
	MinLikes      int
	MinDownloads  int
	FilterSkipped *int
}

const defaultMaxPages = 50000

// ListModels pages through GET /api/models following Link: rel="next".
func (c *Client) ListModels(ctx context.Context, opts ListOpts) ([]Model, error) {
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = defaultMaxPages
	}
	sortKey := strings.TrimSpace(opts.Sort)
	if sortKey == "" {
		sortKey = "downloads"
	}
	dir := strings.TrimSpace(opts.Direction)
	if dir == "" {
		dir = "-1"
	}
	sleep := opts.Sleep
	if sleep <= 0 {
		sleep = 200 * time.Millisecond
	}
	downloadsDesc := strings.EqualFold(sortKey, "downloads") && dir == "-1"

	q := url.Values{}
	if opts.Query != "" {
		q.Set("search", opts.Query)
	}
	if opts.Author != "" {
		q.Set("author", opts.Author)
	}
	if opts.Task != "" && opts.Task != "any" {
		q.Set("pipeline_tag", opts.Task)
	}
	if opts.Library != "" {
		q.Set("library", opts.Library)
	}
	if f := strings.TrimSpace(opts.Filter); f != "" {
		q.Add("filter", f)
	}
	q.Set("sort", sortKey)
	q.Set("direction", dir)
	q.Set("limit", strconv.Itoa(pageSize))
	if opts.Full {
		q.Set("full", "true")
		for _, field := range []string{
			"cardData", "likes", "downloads", "tags", "pipeline_tag",
			"library_name", "gated", "author",
		} {
			q.Add("expand", field)
		}
	}

	path := "/api/models?" + q.Encode()
	var out []Model
	seen := map[string]bool{}

	for page := 0; page < maxPages; page++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		models, next, err := c.getModelsPage(ctx, path)
		if err != nil {
			return out, err
		}
		if c.Rotator != nil {
			c.Rotator.MarkCall()
		}
		if len(models) == 0 {
			break
		}

		if opts.MinDownloads > 0 && downloadsDesc && pageAllBelowDownloads(models, opts.MinDownloads) {
			break
		}

		stopOlder := false
		for _, m := range models {
			id := m.RepoID()
			if id == "" || seen[id] {
				continue
			}
			if opts.MinLikes > 0 && m.Likes < opts.MinLikes {
				if opts.FilterSkipped != nil {
					*opts.FilterSkipped++
				}
				continue
			}
			if opts.MinDownloads > 0 && m.Downloads < int64(opts.MinDownloads) {
				if opts.FilterSkipped != nil {
					*opts.FilterSkipped++
				}
				continue
			}
			if opts.SinceUnix > 0 && strings.EqualFold(sortKey, "lastModified") {
				lm := parseLastModifiedUnix(m.LastModified)
				if lm > 0 && lm < opts.SinceUnix {
					stopOlder = true
					continue
				}
			}
			seen[id] = true
			out = append(out, m)
			if opts.Limit > 0 && len(out) >= opts.Limit {
				return out[:opts.Limit], nil
			}
		}
		if stopOlder || next == "" {
			break
		}
		path = next
		if page+1 < maxPages {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(sleep):
			}
		}
	}
	return out, nil
}

// CountModels pages the Hub like ListModels but only returns how many unique
// repo ids match (no quality floors). Prefer X-Total-Count when the Hub sends it.
func (c *Client) CountModels(ctx context.Context, opts ListOpts) (int, error) {
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 1000
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = 500 // cap ~500k models so CI stays bounded
	}
	sleep := opts.Sleep
	if sleep <= 0 {
		sleep = 100 * time.Millisecond
	}

	q := url.Values{}
	if opts.Author != "" {
		q.Set("author", opts.Author)
	}
	if opts.Task != "" && opts.Task != "any" {
		q.Set("pipeline_tag", opts.Task)
	}
	if opts.Library != "" {
		q.Set("library", opts.Library)
	}
	if f := strings.TrimSpace(opts.Filter); f != "" {
		q.Add("filter", f)
	}
	q.Set("sort", "downloads")
	q.Set("direction", "-1")
	q.Set("limit", strconv.Itoa(pageSize))

	path := "/api/models?" + q.Encode()
	total := 0
	seen := map[string]bool{}

	for page := 0; page < maxPages; page++ {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		models, next, hdrTotal, err := c.getModelsPageCounted(ctx, path)
		if err != nil {
			return total, err
		}
		if c.Rotator != nil {
			c.Rotator.MarkCall()
		}
		if page == 0 && hdrTotal > 0 {
			return hdrTotal, nil
		}
		if len(models) == 0 {
			break
		}
		for _, m := range models {
			id := m.RepoID()
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			total++
		}
		if next == "" {
			break
		}
		path = next
		if page+1 < maxPages {
			select {
			case <-ctx.Done():
				return total, ctx.Err()
			case <-time.After(sleep):
			}
		}
	}
	return total, nil
}

func (c *Client) getModelsPageCounted(ctx context.Context, pathOrURL string) ([]Model, string, int, error) {
	maxAttempts := c.MaxProxyTries
	if maxAttempts <= 0 {
		maxAttempts = 10_000
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if last != nil {
				return nil, "", 0, last
			}
			return nil, "", 0, err
		}
		models, next, hdrTotal, err := c.fetchPageCounted(ctx, pathOrURL)
		if err == nil {
			return models, next, hdrTotal, nil
		}
		last = err
		if !errors.Is(err, ErrRetryable) || c.Rotator == nil {
			return nil, "", 0, err
		}
		c.Rotator.Rotate(fmt.Sprintf("attempt %d/%d: %v", attempt+1, maxAttempts, err))
	}
	return nil, "", 0, fmt.Errorf("exhausted %d proxy tries: %w", maxAttempts, last)
}

func (c *Client) fetchPageCounted(ctx context.Context, pathOrURL string) ([]Model, string, int, error) {
	base := c.BaseURL
	if base == "" {
		base = BaseURL
	}
	reqURL := pathOrURL
	if strings.HasPrefix(pathOrURL, "/") {
		reqURL = base + pathOrURL
	} else if !strings.HasPrefix(pathOrURL, "http://") && !strings.HasPrefix(pathOrURL, "https://") {
		reqURL = base + "/" + strings.TrimPrefix(pathOrURL, "/")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.Header.Set("Accept", "application/json")
	ua := c.UserAgent
	if ua == "" {
		ua = "hfpacks/0.1"
	}
	req.Header.Set("User-Agent", ua)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("%w: %v", ErrRetryable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, "", 0, fmt.Errorf("%w: read body: %v", ErrRetryable, err)
	}
	switch res.StatusCode {
	case http.StatusTooManyRequests, http.StatusForbidden:
		return nil, "", 0, fmt.Errorf("%w: HTTP %d: %s", ErrRetryable, res.StatusCode, trimBody(body))
	case http.StatusUnauthorized:
		return nil, "", 0, fmt.Errorf("huggingface: HTTP %d (check HF_TOKEN): %s", res.StatusCode, trimBody(body))
	}
	if res.StatusCode >= 300 {
		if res.StatusCode >= 500 {
			return nil, "", 0, fmt.Errorf("%w: HTTP %d: %s", ErrRetryable, res.StatusCode, trimBody(body))
		}
		return nil, "", 0, fmt.Errorf("huggingface: HTTP %d: %s", res.StatusCode, trimBody(body))
	}
	var models []Model
	if err := json.Unmarshal(body, &models); err != nil {
		return nil, "", 0, fmt.Errorf("huggingface: decode: %w", err)
	}
	hdrTotal := 0
	if v := strings.TrimSpace(res.Header.Get("X-Total-Count")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			hdrTotal = n
		}
	}
	return models, parseLinkNext(res.Header.Get("Link"), base), hdrTotal, nil
}

func pageAllBelowDownloads(models []Model, minDownloads int) bool {
	if len(models) == 0 || minDownloads <= 0 {
		return false
	}
	thresh := int64(minDownloads)
	for _, m := range models {
		if m.Downloads >= thresh {
			return false
		}
	}
	return true
}

func parseLastModifiedUnix(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Unix()
	}
	return 0
}

func (c *Client) getModelsPage(ctx context.Context, pathOrURL string) ([]Model, string, error) {
	maxAttempts := c.MaxProxyTries
	if maxAttempts <= 0 {
		maxAttempts = 10_000 // practical “keep trying”
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if last != nil {
				return nil, "", last
			}
			return nil, "", err
		}
		models, next, err := c.fetchPage(ctx, pathOrURL)
		if err == nil {
			return models, next, nil
		}
		last = err
		if !errors.Is(err, ErrRetryable) || c.Rotator == nil {
			return nil, "", err
		}
		c.Rotator.Rotate(fmt.Sprintf("attempt %d/%d: %v", attempt+1, maxAttempts, err))
	}
	return nil, "", fmt.Errorf("exhausted %d proxy tries: %w", maxAttempts, last)
}

func (c *Client) fetchPage(ctx context.Context, pathOrURL string) ([]Model, string, error) {
	base := c.BaseURL
	if base == "" {
		base = BaseURL
	}
	reqURL := pathOrURL
	if strings.HasPrefix(pathOrURL, "/") {
		reqURL = base + pathOrURL
	} else if !strings.HasPrefix(pathOrURL, "http://") && !strings.HasPrefix(pathOrURL, "https://") {
		reqURL = base + "/" + strings.TrimPrefix(pathOrURL, "/")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/json")
	ua := c.UserAgent
	if ua == "" {
		ua = "hfpacks/0.1"
	}
	req.Header.Set("User-Agent", ua)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrRetryable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, "", fmt.Errorf("%w: read body: %v", ErrRetryable, err)
	}
	switch res.StatusCode {
	case http.StatusTooManyRequests, http.StatusForbidden:
		return nil, "", fmt.Errorf("%w: HTTP %d: %s", ErrRetryable, res.StatusCode, trimBody(body))
	case http.StatusUnauthorized:
		return nil, "", fmt.Errorf("huggingface: HTTP %d (check HF_TOKEN): %s", res.StatusCode, trimBody(body))
	}
	if res.StatusCode >= 300 {
		if res.StatusCode >= 500 {
			return nil, "", fmt.Errorf("%w: HTTP %d: %s", ErrRetryable, res.StatusCode, trimBody(body))
		}
		return nil, "", fmt.Errorf("huggingface: HTTP %d: %s", res.StatusCode, trimBody(body))
	}
	var models []Model
	if err := json.Unmarshal(body, &models); err != nil {
		return nil, "", fmt.Errorf("huggingface: decode: %w", err)
	}
	return models, parseLinkNext(res.Header.Get("Link"), base), nil
}

func parseLinkNext(linkHeader, base string) string {
	if linkHeader == "" {
		return ""
	}
	parts := strings.Split(linkHeader, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) && !strings.Contains(part, `rel=next`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end <= start {
			continue
		}
		raw := strings.TrimSpace(part[start+1 : end])
		if strings.HasPrefix(raw, base) {
			return strings.TrimPrefix(raw, base)
		}
		return raw
	}
	return ""
}
