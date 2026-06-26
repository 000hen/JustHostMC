package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFabricVersionsReturnsStableGameVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/versions/game" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"version": "25w01a", "stable": false},
			{"version": "1.21.6", "stable": true},
			{"version": "1.21.5", "stable": true},
		})
	}))
	defer srv.Close()

	f := NewFabric(WithFabricMetaBase(srv.URL))
	versions, err := f.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.21.6", "1.21.5"}
	if len(versions) != len(want) {
		t.Fatalf("versions = %v, want %v", versions, want)
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("versions = %v, want %v", versions, want)
		}
	}
}

func TestFabricInstallDownloadsServerJar(t *testing.T) {
	const jar = "fabric server jar"
	var downloaded bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/versions/loader":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"version": "0.16.14", "stable": true},
			})
		case "/versions/installer":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"version": "1.0.3", "stable": true},
			})
		case "/versions/loader/1.21.1/0.16.14/1.0.3/server/jar":
			downloaded = true
			_, _ = w.Write([]byte(jar))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	f := NewFabric(WithFabricMetaBase(srv.URL))
	spec, err := f.Install(context.Background(), dir, "1.21.1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !downloaded {
		t.Fatal("server jar was not downloaded")
	}
	got, err := os.ReadFile(filepath.Join(dir, "server.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != jar {
		t.Fatalf("server.jar = %q, want %q", string(got), jar)
	}
	if spec.JavaMajor != 21 {
		t.Fatalf("JavaMajor = %d, want 21", spec.JavaMajor)
	}
	wantArgs := []string{"-jar", "server.jar", "nogui"}
	if len(spec.Args) != len(wantArgs) {
		t.Fatalf("Args = %v, want %v", spec.Args, wantArgs)
	}
	for i := range wantArgs {
		if spec.Args[i] != wantArgs[i] {
			t.Fatalf("Args = %v, want %v", spec.Args, wantArgs)
		}
	}
}
