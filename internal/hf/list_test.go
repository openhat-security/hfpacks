package hf

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRotator struct {
	client  *http.Client
	rotates int32
	calls   int32
}

func (f *fakeRotator) Client() *http.Client { return f.client }
func (f *fakeRotator) MarkCall()            { atomic.AddInt32(&f.calls, 1) }
func (f *fakeRotator) Rotate(reason string) { atomic.AddInt32(&f.rotates, 1) }

func TestListModelsPaginationAndRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("slow down"))
			return
		}
		models := []Model{{ID: "org/a", Downloads: 1000, Likes: 10}}
		_ = json.NewEncoder(w).Encode(models)
	}))
	defer srv.Close()

	rot := &fakeRotator{client: srv.Client()}
	c := New("", rot)
	c.BaseURL = srv.URL
	c.MaxProxyTries = 5

	models, err := c.ListModels(context.Background(), ListOpts{
		Limit: 10, PageSize: 10, MaxPages: 2, Sleep: time.Millisecond, Full: false,
		MinLikes: 0, MinDownloads: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "org/a" {
		t.Fatalf("%+v", models)
	}
	if atomic.LoadInt32(&rot.rotates) < 1 {
		t.Fatal("expected rotate on 429")
	}
}

func TestParseLinkNext(t *testing.T) {
	got := parseLinkNext(`<https://huggingface.co/api/models?p=2>; rel="next", <https://huggingface.co/api/models?p=9>; rel="last"`, "https://huggingface.co")
	if got != "/api/models?p=2" {
		t.Fatalf("got %q", got)
	}
}
