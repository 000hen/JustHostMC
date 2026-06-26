package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestJavaMajorForMC(t *testing.T) {
	cases := map[string]int{
		"1.21.1":  21,
		"1.21":    21,
		"1.20.6":  21,
		"1.20.5":  21,
		"1.20.4":  17,
		"1.20":    17,
		"1.19.4":  17,
		"1.18.2":  17,
		"1.17.1":  17,
		"1.16.5":  8,
		"1.8.8":   8,
		"24w01a":  21, // snapshot -> newest
		"26.2":    25, // current scheme (no leading 1.) -> Java 25
		"26.1":    25,
		"26.1.2":  25, // modern scheme (no leading 1.) -> Java 25
		"26w01a":  25,
		"garbage": 21,
	}
	for v, want := range cases {
		if got := JavaMajorForMC(v); got != want {
			t.Errorf("JavaMajorForMC(%q) = %d, want %d", v, got, want)
		}
	}
}

// The Fill API (v3) groups versions by series; Versions should flatten and sort
// newest-first, including the current 26.x versions.
func TestPaperVersionsNewestFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/paper" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"project": map[string]any{"id": "paper", "name": "Paper"},
			"versions": map[string][]string{
				"26.2": {"26.2"},
				"26.1": {"26.1.2", "26.1.1"},
				"1.21": {"1.21.1", "1.21"},
				"1.20": {"1.20.4"},
			},
		})
	}))
	defer srv.Close()
	p := NewPaper(WithPaperAPIBase(srv.URL))

	versions, err := p.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"26.2", "26.1.2", "26.1.1", "1.21.1", "1.21", "1.20.4"}
	if len(versions) != len(want) {
		t.Fatalf("Versions = %v, want %v", versions, want)
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("Versions = %v, want %v", versions, want)
		}
	}
}

func TestPaperInstallUsesLatestBuild(t *testing.T) {
	jar := []byte("PAPER-JAR-CONTENTS")
	sum := sha256.Sum256(jar)
	wantSHA := hex.EncodeToString(sum[:])

	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/projects/paper/versions/26.1.2/builds/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 72,
			"downloads": map[string]any{
				"server:default": map[string]any{
					"name":      "paper-26.1.2-72.jar",
					"url":       base + "/dl/paper-26.1.2-72.jar",
					"checksums": map[string]any{"sha256": wantSHA},
				},
			},
		})
	})
	mux.HandleFunc("/dl/paper-26.1.2-72.jar", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(jar) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()

	p := NewPaper(WithPaperAPIBase(srv.URL))
	dir := t.TempDir()
	spec, err := p.Install(context.Background(), dir, "26.1.2", nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if spec.JavaMajor != 25 {
		t.Errorf("JavaMajor = %d, want 25", spec.JavaMajor)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "server.jar"))
	if string(got) != string(jar) {
		t.Errorf("server.jar = %q, want %q", got, jar)
	}
}

func TestPaperInstallRejectsChecksumMismatch(t *testing.T) {
	jar := []byte("PAPER-JAR-CONTENTS")
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/projects/paper/versions/26.1.2/builds/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 72,
			"downloads": map[string]any{
				"server:default": map[string]any{
					"name":      "paper-26.1.2-72.jar",
					"url":       base + "/dl/paper-26.1.2-72.jar",
					"checksums": map[string]any{"sha256": "0000000000000000000000000000000000000000000000000000000000000000"},
				},
			},
		})
	})
	mux.HandleFunc("/dl/paper-26.1.2-72.jar", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(jar) })
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()

	p := NewPaper(WithPaperAPIBase(srv.URL))
	if _, err := p.Install(context.Background(), t.TempDir(), "26.1.2", nil); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestPaperInstallVersionNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	p := NewPaper(WithPaperAPIBase(srv.URL))

	if _, err := p.Install(context.Background(), t.TempDir(), "9.9.9", nil); err == nil {
		t.Fatal("expected error when version has no builds")
	}
}
