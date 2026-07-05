// Package httpcache is a small disk-backed cache for GET responses with
// ETag revalidation. Shop scripts use it (via jhmc.http_cache) so repeated
// browsing of project details/search pages hits the network only for a cheap
// 304 — or not at all while an entry is still fresh.
package httpcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultMaxBytes caps the cache directory size (LRU-evicted past this).
const DefaultMaxBytes = 50 << 20 // 50 MiB

// maxBody caps a single cached response body (matches jhmc.http's limit).
const maxBody = 64 << 20

// entry is the on-disk envelope for one cached response.
type entry struct {
	URL         string    `json:"url"`
	ETag        string    `json:"etag,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
	Status      int       `json:"status"`
	ContentType string    `json:"content_type,omitempty"`
	Body        []byte    `json:"body"` // encoding/json base64-encodes this
}

// Cache is safe for concurrent use.
type Cache struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex
}

// New returns a cache rooted at dir (created lazily). maxBytes <= 0 uses
// DefaultMaxBytes.
func New(dir string, maxBytes int64) *Cache {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Cache{dir: dir, maxBytes: maxBytes}
}

// Result is a cached or freshly fetched response.
type Result struct {
	Status  int
	Body    []byte
	Headers http.Header
	// Cached reports whether Body was served from disk (fresh-TTL hit or 304).
	Cached bool
}

// Get performs a GET with disk caching. Entries younger than ttl are returned
// without touching the network; older entries are revalidated with
// If-None-Match when an ETag was stored (a 304 refreshes the entry). Only
// 200 responses are cached. headers are extra request headers (User-Agent,
// x-api-key, ...); they are not part of the cache key, so callers must not
// share one Cache across differently-authorized views of the same URL.
func (c *Cache) Get(ctx context.Context, client *http.Client, url string, headers map[string]string, ttl time.Duration) (Result, error) {
	cached, _ := c.load(url)

	if cached != nil && ttl > 0 && time.Since(cached.FetchedAt) < ttl {
		return cached.result(true), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cached != nil && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && cached != nil {
		cached.FetchedAt = time.Now()
		_ = c.store(cached) // refresh freshness window; best-effort
		return cached.result(true), nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Result{}, fmt.Errorf("GET %s: read body: %w", url, err)
	}
	res := Result{Status: resp.StatusCode, Body: body, Headers: resp.Header}
	if resp.StatusCode == http.StatusOK {
		_ = c.store(&entry{
			URL:         url,
			ETag:        resp.Header.Get("Etag"),
			FetchedAt:   time.Now(),
			Status:      resp.StatusCode,
			ContentType: resp.Header.Get("Content-Type"),
			Body:        body,
		})
	}
	return res, nil
}

func (e *entry) result(cached bool) Result {
	h := http.Header{}
	if e.ContentType != "" {
		h.Set("Content-Type", e.ContentType)
	}
	if e.ETag != "" {
		h.Set("Etag", e.ETag)
	}
	return Result{Status: e.Status, Body: e.Body, Headers: h, Cached: cached}
}

func (c *Cache) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:16])+".json")
}

func (c *Cache) load(url string) (*entry, error) {
	if c.dir == "" { // unwired cache: network-only passthrough
		return nil, os.ErrNotExist
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := os.ReadFile(c.path(url))
	if err != nil {
		return nil, err
	}
	var e entry
	if err := json.Unmarshal(b, &e); err != nil || e.URL != url {
		return nil, fmt.Errorf("corrupt cache entry")
	}
	return &e, nil
}

func (c *Cache) store(e *entry) error {
	if c.dir == "" { // unwired cache: network-only passthrough
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp := c.path(e.URL) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path(e.URL)); err != nil {
		return err
	}
	c.evictLocked()
	return nil
}

// evictLocked drops the oldest entries until the directory fits maxBytes.
func (c *Cache) evictLocked() {
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type fileInfo struct {
		path string
		size int64
		mod  time.Time
	}
	var files []fileInfo
	var total int64
	for _, de := range ents {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		fi, err := de.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{filepath.Join(c.dir, de.Name()), fi.Size(), fi.ModTime()})
		total += fi.Size()
	}
	if total <= c.maxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= c.maxBytes {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
