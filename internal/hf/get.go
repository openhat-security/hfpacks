package hf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Get fetches one model card by org/name (or Hub URL). Upserts callers use this
// for targeted index updates without a full category crawl.
func (c *Client) Get(ctx context.Context, repoID string) (*Model, error) {
	repoID = normalizeRepoID(repoID)
	if repoID == "" {
		return nil, fmt.Errorf("model id is required (org/name)")
	}
	path := "/api/models/" + repoID

	maxAttempts := c.MaxProxyTries
	if maxAttempts <= 0 {
		maxAttempts = 10_000
	}
	if c.Rotator == nil {
		maxAttempts = 1
	}

	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if last != nil {
				return nil, last
			}
			return nil, err
		}
		m, err := c.fetchModel(ctx, path, repoID)
		if err == nil {
			if c.Rotator != nil {
				c.Rotator.MarkCall()
			}
			return m, nil
		}
		last = err
		if !errors.Is(err, ErrRetryable) || c.Rotator == nil {
			return nil, err
		}
		c.Rotator.Rotate(fmt.Sprintf("get attempt %d/%d: %v", attempt+1, maxAttempts, err))
	}
	return nil, fmt.Errorf("exhausted %d proxy tries: %w", maxAttempts, last)
}

func (c *Client) fetchModel(ctx context.Context, path, repoID string) (*Model, error) {
	base := c.BaseURL
	if base == "" {
		base = BaseURL
	}
	reqURL := base + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("%w: %v", ErrRetryable, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrRetryable, err)
	}
	switch res.StatusCode {
	case http.StatusTooManyRequests, http.StatusForbidden:
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrRetryable, res.StatusCode, trimBody(body))
	case http.StatusNotFound:
		return nil, fmt.Errorf("huggingface: model %q not found", repoID)
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("huggingface: HTTP %d (check HF_TOKEN): %s", res.StatusCode, trimBody(body))
	}
	if res.StatusCode >= 300 {
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("%w: HTTP %d: %s", ErrRetryable, res.StatusCode, trimBody(body))
		}
		return nil, fmt.Errorf("huggingface: HTTP %d: %s", res.StatusCode, trimBody(body))
	}
	var m Model
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("huggingface: decode: %w", err)
	}
	if m.ID == "" {
		m.ID = repoID
	}
	return &m, nil
}

func normalizeRepoID(repoID string) string {
	repoID = strings.TrimSpace(repoID)
	repoID = strings.TrimPrefix(repoID, "https://huggingface.co/")
	repoID = strings.TrimPrefix(repoID, "http://huggingface.co/")
	return strings.Trim(repoID, "/")
}
