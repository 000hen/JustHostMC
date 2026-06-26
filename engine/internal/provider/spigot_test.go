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

// TestSpigotVersionsReturnsReleases verifies that Versions() returns only
// release versions from the Mojang manifest, in the original order (newest
// first, as Mojang delivers them).
func TestSpigotVersionsReturnsReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"versions": []map[string]string{
				{"id": "26.2", "type": "release"},
				{"id": "26w25a", "type": "snapshot"},
				{"id": "26.1.2", "type": "release"},
				{"id": "1.21.1", "type": "release"},
				{"id": "24w01a", "type": "snapshot"},
				{"id": "1.20.4", "type": "release"},
			},
		})
	}))
	defer srv.Close()

	s := NewSpigot(nil, WithSpigotManifestURL(srv.URL))
	versions, err := s.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"26.2", "26.1.2", "1.21.1", "1.20.4"}
	if len(versions) != len(want) {
		t.Fatalf("Versions = %v, want %v", versions, want)
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("Versions[%d] = %q, want %q", i, versions[i], want[i])
		}
	}
}

// TestSpigotVersionsManifestError verifies that a non-200 manifest response
// returns an error.
func TestSpigotVersionsManifestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()

	s := NewSpigot(nil, WithSpigotManifestURL(srv.URL))
	if _, err := s.Versions(context.Background()); err == nil {
		t.Fatal("expected error from broken manifest")
	}
}

// TestFindSpigotJar verifies that findSpigotJar picks the right jar and skips
// shaded/sources variants.
func TestFindSpigotJar(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"spigot-1.21.1-R0.1-SNAPSHOT-shaded.jar",
		"spigot-1.21.1-R0.1-SNAPSHOT.jar",
		"BuildTools.jar",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("JAR"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := findSpigotJar(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "spigot-1.21.1-R0.1-SNAPSHOT.jar" {
		t.Errorf("findSpigotJar = %q, want spigot-1.21.1-R0.1-SNAPSHOT.jar", got)
	}
}

// TestFindSpigotJarFallbackCraftBukkit verifies fallback to craftbukkit-*.jar.
func TestFindSpigotJarFallbackCraftBukkit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "craftbukkit-1.8.8.jar"), []byte("JAR"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := findSpigotJar(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "craftbukkit-1.8.8.jar" {
		t.Errorf("findSpigotJar = %q, want craftbukkit-1.8.8.jar", got)
	}
}

// TestFindSpigotJarMissing verifies that an error is returned when no jar is found.
func TestFindSpigotJarMissing(t *testing.T) {
	if _, err := findSpigotJar(t.TempDir()); err == nil {
		t.Fatal("expected error when no spigot jar exists")
	}
}
