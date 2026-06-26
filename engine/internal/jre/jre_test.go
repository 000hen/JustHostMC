package jre

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/000hen/justhostmc/engine/internal/provider"
)

// makeJREZip builds a tiny archive shaped like an Adoptium JRE (java.exe nested
// under <top>/bin/).
func makeJREZip(t *testing.T, javaContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("jdk-21.0.4+7-jre/bin/java.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(javaContent)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newAdoptiumStub(t *testing.T, zipBytes []byte) string {
	t.Helper()
	sum := sha256.Sum256(zipBytes)
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/assets/latest/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"binary": map[string]any{"package": map[string]any{
				"link":     base + "/jre.zip",
				"checksum": hex.EncodeToString(sum[:]),
				"name":     "OpenJDK-jre.zip",
			}}},
		})
	})
	mux.HandleFunc("/jre.zip", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBytes)
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return base
}

func TestEnsureJREDownloadsExtractsAndReturnsJavaPath(t *testing.T) {
	zipBytes := makeJREZip(t, "fake-java-binary")
	base := newAdoptiumStub(t, zipBytes)
	cache := t.TempDir()
	m := NewManager(cache, WithAPIBase(base), WithHTTPClient(http.DefaultClient))

	java, err := m.EnsureJRE(context.Background(), 21, nil)
	if err != nil {
		t.Fatalf("EnsureJRE: %v", err)
	}
	if !strings.EqualFold(filepath.Base(java), "java.exe") {
		t.Errorf("java path = %q, want it to end in java.exe", java)
	}
	if !strings.HasPrefix(java, filepath.Join(cache, "21")) {
		t.Errorf("java path = %q, want it under cache/21", java)
	}
	content, _ := os.ReadFile(java)
	if string(content) != "fake-java-binary" {
		t.Errorf("java content = %q, want fake-java-binary", content)
	}
}

func TestEnsureJRECacheHitSkipsNetwork(t *testing.T) {
	cache := t.TempDir()
	binDir := filepath.Join(cache, "17", "jdk-17-jre", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cached := filepath.Join(binDir, "java.exe")
	if err := os.WriteFile(cached, []byte("already here"), 0o755); err != nil {
		t.Fatal(err)
	}

	failing := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("network call on cache hit: %s", r.URL)
		return nil, errors.New("must not be called")
	})}
	m := NewManager(cache, WithAPIBase("http://invalid.invalid"), WithHTTPClient(failing))

	java, err := m.EnsureJRE(context.Background(), 17, nil)
	if err != nil {
		t.Fatalf("EnsureJRE (cache hit): %v", err)
	}
	if java != cached {
		t.Errorf("java = %q, want cached %q", java, cached)
	}
}

func TestEnsureJREUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]")) // no assets
	}))
	defer srv.Close()
	m := NewManager(t.TempDir(), WithAPIBase(srv.URL))

	_, err := m.EnsureJRE(context.Background(), 99, nil)
	if !errors.Is(err, ErrJREUnavailable) {
		t.Fatalf("err = %v, want ErrJREUnavailable", err)
	}
}

func TestEnsureJREChecksumMismatch(t *testing.T) {
	zipBytes := makeJREZip(t, "data")
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/assets/latest/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"binary": map[string]any{"package": map[string]any{
				"link": base + "/jre.zip", "checksum": "deadbeef", "name": "x.zip",
			}}},
		})
	})
	mux.HandleFunc("/jre.zip", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(zipBytes) })
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()
	m := NewManager(t.TempDir(), WithAPIBase(base))

	_, err := m.EnsureJRE(context.Background(), 21, nil)
	if !errors.Is(err, provider.ErrChecksumMismatch) {
		t.Fatalf("err = %v, want ErrChecksumMismatch", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
