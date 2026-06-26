package dl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadWritesFileComputesSumAndReportsProgress(t *testing.T) {
	payload := []byte("hello minecraft server jar")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "26")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "nested", "server.jar")
	var lastDone, lastTotal int64
	calls := 0

	sum, n, err := Download(context.Background(), srv.Client(), srv.URL, dest, sha256.New(),
		func(done, total int64) { lastDone, lastTotal = done, total; calls++ })
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	if n != int64(len(payload)) {
		t.Errorf("n = %d, want %d", n, len(payload))
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(payload) {
		t.Errorf("file content = %q, want %q", got, payload)
	}
	want := sha256.Sum256(payload)
	if sum != hex.EncodeToString(want[:]) {
		t.Errorf("sum = %s, want %s", sum, hex.EncodeToString(want[:]))
	}
	if calls == 0 || lastDone != int64(len(payload)) || lastTotal != 26 {
		t.Errorf("progress: calls=%d lastDone=%d lastTotal=%d", calls, lastDone, lastTotal)
	}
}

func TestDownloadRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := Download(context.Background(), srv.Client(), srv.URL, filepath.Join(t.TempDir(), "x"), nil, nil)
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}
