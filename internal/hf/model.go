package hf

import (
	"net/http"
	"strings"
)

const BaseURL = "https://huggingface.co"

// Rotator is satisfied by *proxy.Pool.
type Rotator interface {
	Client() *http.Client
	MarkCall()
	Rotate(reason string)
}

// Client talks to the Hugging Face Hub API.
type Client struct {
	HTTP          *http.Client
	Token         string
	BaseURL       string
	Rotator       Rotator
	UserAgent     string
	MaxProxyTries int // per Hub page; <=0 means keep trying (cap 10_000)
}

// New builds a client. When rotator is non-nil, HTTP is taken from it on each request.
func New(token string, rotator Rotator) *Client {
	c := &Client{
		Token:     token,
		BaseURL:   BaseURL,
		Rotator:   rotator,
		UserAgent: "hfpacks/0.1",
	}
	if rotator != nil {
		c.HTTP = rotator.Client()
	} else {
		c.HTTP = &http.Client{}
	}
	return c
}

func (c *Client) httpClient() *http.Client {
	if c.Rotator != nil {
		return c.Rotator.Client()
	}
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Model is a slim Hub model card used for indexing.
type Model struct {
	ID           string         `json:"id"`
	ModelID      string         `json:"modelId"`
	Author       string         `json:"author"`
	PipelineTag  string         `json:"pipeline_tag"`
	LibraryName  string         `json:"library_name"`
	Tags         []string       `json:"tags"`
	Downloads    int64          `json:"downloads"`
	Likes        int            `json:"likes"`
	LastModified string         `json:"lastModified"`
	CardData     map[string]any `json:"cardData"`
	Description  string         `json:"description"`
}

func (m Model) RepoID() string {
	if m.ID != "" {
		return m.ID
	}
	return m.ModelID
}

func (m Model) License() string {
	for _, tag := range m.Tags {
		if strings.HasPrefix(tag, "license:") {
			return strings.TrimPrefix(tag, "license:")
		}
	}
	if m.CardData != nil {
		if lic, ok := m.CardData["license"].(string); ok && lic != "" {
			return lic
		}
	}
	return ""
}

// CardDescription returns a short description for the index.
func (m Model) CardDescription() string {
	if s := strings.TrimSpace(m.Description); s != "" {
		return clipDesc(s)
	}
	if m.CardData == nil {
		return ""
	}
	for _, key := range []string{"description", "summary", "text"} {
		if s, ok := m.CardData[key].(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				return clipDesc(s)
			}
		}
	}
	return ""
}

func clipDesc(s string) string {
	if len(s) > 2000 {
		return s[:2000]
	}
	return s
}

func trimBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
