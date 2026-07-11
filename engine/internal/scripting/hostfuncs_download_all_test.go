package scripting

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/000hen/justhostmc/engine/internal/provider"
)

// downloadAllScript wraps a Lua install() body that calls jhmc.download_all.
func downloadAllScript(body string) string {
	return fmt.Sprintf(`
meta = { id = "batch", name = "Batch", mod_layout = "mods",
  permissions = { { kind = "network", reason = "test" } } }
function versions() return {} end
function install(ctx)
%s
  return { java_major = 21, args = { "-jar", "s.jar" } }
end
`, body)
}

func installBatch(t *testing.T, script, dir string, progress func(provider.Progress)) error {
	t.Helper()
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	e, err := r.AddSource(context.Background(), script, true) // builtin → network granted
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	_, err = e.Provider.Install(context.Background(), dir, "1.0", progress)
	return err
}

// TestDownloadAllDownloadsConcurrently proves N slow files complete in roughly
// one slot's time, land at their dests with the right bytes, and emit one
// completion progress line per file.
func TestDownloadAllDownloadsConcurrently(t *testing.T) {
	const n = 6
	var inflight, peak atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/f/", func(w http.ResponseWriter, r *http.Request) {
		cur := inflight.Add(1)
		defer inflight.Add(-1)
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write([]byte("body-" + strings.TrimPrefix(r.URL.Path, "/f/")))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	items := make([]string, 0, n)
	for i := range n {
		items = append(items, fmt.Sprintf(`{ dest = "mods/f%d.txt", url = %q }`, i, fmt.Sprintf("%s/f/%d", srv.URL, i)))
	}
	script := downloadAllScript(`  jhmc.download_all({ ` + strings.Join(items, ", ") + ` })`)

	dir := t.TempDir()
	var mu sync.Mutex
	var logs []string
	start := time.Now()
	err := installBatch(t, script, dir, func(p provider.Progress) {
		if p.LogLine != "" {
			mu.Lock()
			logs = append(logs, p.LogLine)
			mu.Unlock()
		}
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	for i := range n {
		b, err := os.ReadFile(filepath.Join(dir, "mods", fmt.Sprintf("f%d.txt", i)))
		if err != nil || string(b) != fmt.Sprintf("body-%d", i) {
			t.Errorf("f%d.txt content = %q err=%v", i, b, err)
		}
	}
	if peak.Load() < 2 {
		t.Errorf("peak concurrency = %d, want >= 2", peak.Load())
	}
	if elapsed > 700*time.Millisecond {
		t.Errorf("elapsed = %v, want well under serial 6*150ms + overhead", elapsed)
	}
	if len(logs) != n {
		t.Errorf("completion log lines = %d (%v), want %d", len(logs), logs, n)
	}
}

// TestDownloadAllResolveChain proves a resolve item fetches the JSON endpoint
// (with headers) and downloads the URL in its data field.
func TestDownloadAllResolveChain(t *testing.T) {
	mux := http.NewServeMux()
	var gotKey atomic.Value
	var base string
	mux.HandleFunc("/resolve", func(w http.ResponseWriter, r *http.Request) {
		gotKey.Store(r.Header.Get("x-api-key"))
		fmt.Fprintf(w, `{"data":%q}`, base+"/real")
	})
	mux.HandleFunc("/real", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("resolved-body"))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)

	script := downloadAllScript(fmt.Sprintf(
		`  jhmc.download_all({ { dest = "mods/r.jar", resolve = { url = %q, headers = { ["x-api-key"] = "sekrit" } } } })`,
		srv.URL+"/resolve"))
	dir := t.TempDir()
	if err := installBatch(t, script, dir, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "mods", "r.jar")); string(b) != "resolved-body" {
		t.Errorf("resolved file content = %q", b)
	}
	if gotKey.Load() != "sekrit" {
		t.Errorf("resolve header = %v, want sekrit", gotKey.Load())
	}
}

// TestDownloadAllFirstErrorCancels proves one failing item fails the whole
// call, names the dest, and cancels in-flight downloads instead of waiting for
// them to finish.
func TestDownloadAllFirstErrorCancels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/bad", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
		case <-r.Context().Done():
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	script := downloadAllScript(fmt.Sprintf(`  jhmc.download_all({
    { dest = "mods/slow.jar", url = %q },
    { dest = "mods/bad.jar", url = %q },
  })`, srv.URL+"/slow", srv.URL+"/bad"))

	start := time.Now()
	err := installBatch(t, script, t.TempDir(), nil)
	if err == nil {
		t.Fatal("install succeeded, want error")
	}
	if !strings.Contains(err.Error(), "bad.jar") {
		t.Errorf("error %q does not name the failing dest", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("elapsed = %v; in-flight download was not canceled", elapsed)
	}
}

// TestDownloadAllChecksumMismatch proves a wrong sha1 fails the batch with the
// checksum sentinel.
func TestDownloadAllChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("actual-bytes"))
	}))
	t.Cleanup(srv.Close)

	wrong := sha1.Sum([]byte("other-bytes"))
	script := downloadAllScript(fmt.Sprintf(
		`  jhmc.download_all({ { dest = "mods/c.jar", url = %q, sha1 = %q } })`,
		srv.URL, hex.EncodeToString(wrong[:])))
	err := installBatch(t, script, t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
}

// TestDownloadAllProgressFractions proves fractions are completed/total.
func TestDownloadAllProgressFractions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	t.Cleanup(srv.Close)

	script := downloadAllScript(fmt.Sprintf(`  jhmc.download_all({
    { dest = "a.txt", url = %[1]q }, { dest = "b.txt", url = %[1]q },
    { dest = "c.txt", url = %[1]q }, { dest = "d.txt", url = %[1]q },
  }, { concurrency = 1 })`, srv.URL))

	var fractions []float64
	err := installBatch(t, script, t.TempDir(), func(p provider.Progress) {
		if p.LogLine != "" && p.Fraction >= 0 {
			fractions = append(fractions, p.Fraction)
		}
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	want := []float64{0.25, 0.5, 0.75, 1}
	if len(fractions) != len(want) {
		t.Fatalf("fractions = %v, want %v", fractions, want)
	}
	for i := range want {
		if fractions[i] != want[i] {
			t.Errorf("fractions[%d] = %v, want %v (all: %v)", i, fractions[i], want[i], fractions)
		}
	}
}
