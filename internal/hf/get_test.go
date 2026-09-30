package hf

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGetModel(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/api/models/org/cool" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("slow down"))
			return
		}
		_ = json.NewEncoder(w).Encode(Model{
			ID: "org/cool", Downloads: 42, Likes: 7, PipelineTag: "text-generation",
		})
	}))
	defer srv.Close()

	rot := &fakeRotator{client: srv.Client()}
	c := New("", rot)
	c.BaseURL = srv.URL
	c.MaxProxyTries = 5

	m, err := c.Get(context.Background(), "https://huggingface.co/org/cool")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "org/cool" || m.Downloads != 42 {
		t.Fatalf("%+v", m)
	}
	if atomic.LoadInt32(&rot.rotates) < 1 {
		t.Fatal("expected rotate on 429")
	}
}

func TestGetModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := New("", nil)
	c.HTTP = srv.Client()
	c.BaseURL = srv.URL

	_, err := c.Get(context.Background(), "missing/model")
	if err == nil {
		t.Fatal("expected error")
	}
}
