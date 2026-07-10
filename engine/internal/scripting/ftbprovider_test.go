package scripting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// newFTBProvider loads the builtin providers on a host whose HTTP traffic is
// redirected to handler, and returns the ftb entry.
func newFTBProvider(t *testing.T, handler http.Handler) *Entry {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: rewriteTransport{target: u}}
	r := NewRegistry(NewHost(client, nil, nil), nil)
	if err := LoadBuiltins(context.Background(), r); err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	e, ok := r.Get("ftb")
	if !ok {
		t.Fatal("ftb provider not registered")
	}
	return e
}

func TestFTBProviderIsHiddenWithKeyConfig(t *testing.T) {
	e := newFTBProvider(t, http.NotFoundHandler())
	if !e.Meta.Hidden {
		t.Fatal("ftb provider should be hidden")
	}
	if e.Meta.ModLayout != "mods" {
		t.Fatalf("mod_layout = %q, want mods", e.Meta.ModLayout)
	}
	var hasKey bool
	for _, c := range e.Meta.Config {
		if c.Key == "curseforge_api_key" && c.Type == mcmanagerv1.ConfigOptionType_CONFIG_OPTION_SECRET {
			hasKey = true
		}
	}
	if !hasKey {
		t.Fatalf("ftb should declare a curseforge_api_key secret option, config=%+v", e.Meta.Config)
	}
}

func TestFTBProviderVersionsEmpty(t *testing.T) {
	e := newFTBProvider(t, http.NotFoundHandler())
	vs, err := e.Provider.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("versions = %v, want empty", vs)
	}
}

func TestFTBProviderRejectsBadVersionID(t *testing.T) {
	e := newFTBProvider(t, http.NotFoundHandler())
	if _, err := e.Provider.Install(context.Background(), t.TempDir(), "no-slash", nil); err == nil ||
		!strings.Contains(err.Error(), "invalid modpack version id") {
		t.Fatalf("want invalid-version error, got %v", err)
	}
}

// ftbManifestHandler serves one modpack version manifest at /modpack/1/2.
func ftbManifestHandler(manifest map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/modpack/1/2") {
			_ = json.NewEncoder(w).Encode(manifest)
			return
		}
		http.NotFound(w, r)
	})
}

func TestFTBProviderUnsupportedLoader(t *testing.T) {
	e := newFTBProvider(t, ftbManifestHandler(map[string]any{
		"targets": []map[string]any{
			{"type": "game", "version": "1.20.1"},
			{"type": "modloader", "name": "quilt", "version": "0.1"},
		},
		"files": []any{},
	}))
	if _, err := e.Provider.Install(context.Background(), t.TempDir(), "1/2", nil); err == nil ||
		!strings.Contains(err.Error(), "unsupported modloader") {
		t.Fatalf("want unsupported-modloader error, got %v", err)
	}
}

func TestFTBProviderMissingTargets(t *testing.T) {
	e := newFTBProvider(t, ftbManifestHandler(map[string]any{
		"targets": []map[string]any{{"type": "game", "version": "1.20.1"}},
		"files":   []any{},
	}))
	if _, err := e.Provider.Install(context.Background(), t.TempDir(), "1/2", nil); err == nil ||
		!strings.Contains(err.Error(), "modloader target") {
		t.Fatalf("want missing-modloader error, got %v", err)
	}
}

func TestFTBProviderCurseForgeNeedsKey(t *testing.T) {
	// A CurseForge-hosted file with no configured key must fail with a clear,
	// actionable message rather than silently skipping the mod.
	e := newFTBProvider(t, ftbManifestHandler(map[string]any{
		"targets": []map[string]any{
			{"type": "game", "version": "1.20.1"},
			{"type": "modloader", "name": "forge", "version": "47.2.0"},
		},
		"files": []map[string]any{
			{"name": "secret.jar", "path": "./mods/", "curseforge": map[string]any{"project": 1, "file": 2}},
		},
	}))
	if _, err := e.Provider.Install(context.Background(), t.TempDir(), "1/2", nil); err == nil ||
		!strings.Contains(err.Error(), "CurseForge API key") {
		t.Fatalf("want CurseForge-key error, got %v", err)
	}
}

func TestFallbackConfigReader(t *testing.T) {
	base := NewConfigStore(filepath.Join(t.TempDir(), "c.json"))
	reader := NewFallbackConfigReader(base, "ftb", "curseforge_api_key", func() string { return "shopkey" })

	if got := reader.Values("ftb")["curseforge_api_key"]; got != "shopkey" {
		t.Fatalf("unset key should fall back, got %q", got)
	}
	if err := base.Set("ftb", "curseforge_api_key", "userkey"); err != nil {
		t.Fatal(err)
	}
	if got := reader.Values("ftb")["curseforge_api_key"]; got != "userkey" {
		t.Fatalf("stored value should win, got %q", got)
	}
	if _, ok := reader.Values("other")["curseforge_api_key"]; ok {
		t.Fatal("fallback leaked to a different id")
	}
}
