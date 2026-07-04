package httpcache

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newServer returns a test server that serves body with etag and counts
// full responses vs 304s.
func newServer(t *testing.T, etag string, body *string, full, notModified *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag && etag != "" {
			*notModified++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		*full++
		w.Header().Set("Etag", etag)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(*body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGetCachesAndRevalidates(t *testing.T) {
	body := `{"ok":true}`
	var full, notMod int
	srv := newServer(t, `"v1"`, &body, &full, &notMod)
	c := New(t.TempDir(), 0)
	ctx := context.Background()

	// First fetch: full response, cached.
	res, err := c.Get(ctx, srv.Client(), srv.URL, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached || res.Status != 200 || string(res.Body) != body {
		t.Fatalf("first fetch: got cached=%v status=%d body=%q", res.Cached, res.Status, res.Body)
	}

	// Within TTL: served from disk, no request at all.
	res, err = c.Get(ctx, srv.Client(), srv.URL, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cached || full != 1 {
		t.Fatalf("ttl hit: cached=%v full=%d", res.Cached, full)
	}

	// TTL zero forces revalidation: expect a 304, body still served.
	res, err = c.Get(ctx, srv.Client(), srv.URL, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cached || notMod != 1 || string(res.Body) != body {
		t.Fatalf("revalidate: cached=%v notMod=%d body=%q", res.Cached, notMod, res.Body)
	}
}

func TestGetRefetchesOnChangedEtag(t *testing.T) {
	body := "one"
	var full, notMod int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never matches the stored etag: always a fresh 200.
		full++
		w.Header().Set("Etag", `"v`+string(rune('0'+full))+`"`)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(t.TempDir(), 0)
	ctx := context.Background()
	if _, err := c.Get(ctx, srv.Client(), srv.URL, nil, 0); err != nil {
		t.Fatal(err)
	}
	body = "two"
	res, err := c.Get(ctx, srv.Client(), srv.URL, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached || string(res.Body) != "two" || full != 2 {
		t.Fatalf("changed etag: cached=%v body=%q full=%d notMod=%d", res.Cached, res.Body, full, notMod)
	}
}

func TestNon200NotCached(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(t.TempDir(), 0)
	ctx := context.Background()
	for i := range 2 {
		res, err := c.Get(ctx, srv.Client(), srv.URL, nil, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if res.Status != 404 || res.Cached {
			t.Fatalf("call %d: status=%d cached=%v", i, res.Status, res.Cached)
		}
	}
	if calls != 2 {
		t.Fatalf("404 was cached: calls=%d", calls)
	}
}

func TestRequestHeadersForwarded(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(t.TempDir(), 0)
	if _, err := c.Get(context.Background(), srv.Client(), srv.URL, map[string]string{"x-api-key": "sekret"}, 0); err != nil {
		t.Fatal(err)
	}
	if got != "sekret" {
		t.Fatalf("x-api-key not forwarded: %q", got)
	}
}
