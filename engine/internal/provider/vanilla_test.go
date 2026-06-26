package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newMojangStub serves a minimal manifest, per-version detail, and server jar.
func newMojangStub(t *testing.T, jar []byte, javaMajor int) (*httptest.Server, string) {
	t.Helper()
	sum := sha1.Sum(jar)
	var base string

	mux := http.NewServeMux()
	mux.HandleFunc("/manifest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"latest": map[string]string{"release": "1.21", "snapshot": "24w01a"},
			"versions": []map[string]any{
				{"id": "1.21", "type": "release", "url": base + "/v/1.21"},
				{"id": "24w01a", "type": "snapshot", "url": base + "/v/24w01a"},
			},
		})
	})
	mux.HandleFunc("/v/1.21", func(w http.ResponseWriter, r *http.Request) {
		detail := map[string]any{
			"downloads": map[string]any{
				"server": map[string]any{"url": base + "/server.jar", "size": len(jar), "sha1": hex.EncodeToString(sum[:])},
			},
		}
		if javaMajor > 0 {
			detail["javaVersion"] = map[string]any{"majorVersion": javaMajor}
		}
		_ = json.NewEncoder(w).Encode(detail)
	})
	mux.HandleFunc("/server.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jar)
	})

	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv, base + "/manifest"
}

func TestVanillaVersions(t *testing.T) {
	_, manifestURL := newMojangStub(t, []byte("JAR"), 21)
	v := NewVanilla(WithManifestURL(manifestURL))

	versions, err := v.Versions(context.Background())
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	want := []string{"1.21", "24w01a"}
	if len(versions) != len(want) || versions[0] != want[0] || versions[1] != want[1] {
		t.Errorf("Versions = %v, want %v", versions, want)
	}
}

func TestVanillaInstallDownloadsJarAndReturnsLaunchSpec(t *testing.T) {
	jar := []byte("FAKE-SERVER-JAR-CONTENT")
	_, manifestURL := newMojangStub(t, jar, 21)
	v := NewVanilla(WithManifestURL(manifestURL))
	dir := t.TempDir()

	var steps []string
	var lastFraction float64
	spec, err := v.Install(context.Background(), dir, "1.21", func(p Progress) {
		if p.Step != "" {
			steps = append(steps, p.Step)
		}
		if p.Fraction >= 0 {
			lastFraction = p.Fraction
		}
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if spec.JavaMajor != 21 {
		t.Errorf("JavaMajor = %d, want 21", spec.JavaMajor)
	}
	wantArgs := []string{"-jar", "server.jar", "nogui"}
	if len(spec.Args) != 3 || spec.Args[0] != wantArgs[0] || spec.Args[1] != wantArgs[1] || spec.Args[2] != wantArgs[2] {
		t.Errorf("Args = %v, want %v", spec.Args, wantArgs)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "server.jar"))
	if string(got) != string(jar) {
		t.Errorf("server.jar = %q, want %q", got, jar)
	}
	if lastFraction != 1.0 {
		t.Errorf("final fraction = %v, want 1.0", lastFraction)
	}
	// The download step must have been announced before completion.
	foundDownloading := false
	for _, s := range steps {
		if s == "install.progress.downloading_server" {
			foundDownloading = true
		}
	}
	if !foundDownloading {
		t.Errorf("steps = %v, missing downloading_server", steps)
	}
}

func TestVanillaInstallDefaultsJavaMajorWhenAbsent(t *testing.T) {
	_, manifestURL := newMojangStub(t, []byte("JAR"), 0) // omit javaVersion
	v := NewVanilla(WithManifestURL(manifestURL))

	spec, err := v.Install(context.Background(), t.TempDir(), "1.21", nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if spec.JavaMajor != defaultVanillaJavaMajor {
		t.Errorf("JavaMajor = %d, want %d (default)", spec.JavaMajor, defaultVanillaJavaMajor)
	}
}

func TestVanillaInstallUnknownVersion(t *testing.T) {
	_, manifestURL := newMojangStub(t, []byte("JAR"), 21)
	v := NewVanilla(WithManifestURL(manifestURL))

	_, err := v.Install(context.Background(), t.TempDir(), "9.99", nil)
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("err = %v, want ErrVersionNotFound", err)
	}
}
