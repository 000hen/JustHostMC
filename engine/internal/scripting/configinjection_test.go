package scripting

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// cfgShopSrc echoes its typed config back through a search result so tests can
// assert what the adapter injected: title = api_key, summary = region.
const cfgShopSrc = `
meta = {
  id = "cfgshop",
  name = "Cfg Shop",
  version = "1.0",
  needs_key = true,
  permissions = { { kind = "network", reason = "x" } },
  config = {
    { key = "region", type = "string", name = "Region", default = "us" },
    { key = "api_key", type = "secret", name = "API key" },
  },
}
function home(ctx) return { sections = {} } end
function search(ctx)
  return { projects = { { project_id = "p",
    title = ctx.config.api_key or "",
    summary = ctx.config.region or "" } }, total = 0 }
end
function detail(ctx) return { project = { project_id = "p" } } end
function versions(ctx) return { versions = {} } end
function resolve_file(ctx) error("not distributable") end
`

func cfgShop(t *testing.T, keyFn func(string) string) (*LuaShop, *ConfigStore) {
	t.Helper()
	cs := NewConfigStore(filepath.Join(t.TempDir(), "shop-config.json"))
	ss := NewShopSet(NewHost(nil, nil, nil), nil, keyFn)
	ss.SetConfigStore(cs)
	s, err := ss.AddSource(context.Background(), cfgShopSrc, true)
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	return s, cs
}

// echoedConfig runs search and returns (api_key, region) as the script saw them.
func echoedConfig(t *testing.T, s *LuaShop) (apiKey, region string) {
	t.Helper()
	page, err := s.Search(context.Background(), ShopQuery{Query: "q", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Projects) != 1 {
		t.Fatalf("projects: %+v", page.Projects)
	}
	return page.Projects[0].Title, page.Projects[0].Summary
}

func TestShopConfigDefaultsAndOverride(t *testing.T) {
	s, cs := cfgShop(t, func(string) string { return "chainkey" })

	// Declared default is applied when nothing is stored.
	if _, region := echoedConfig(t, s); region != "us" {
		t.Fatalf("default region = %q, want us", region)
	}
	// A stored override wins over the declared default.
	if err := cs.Set("cfgshop", "region", "eu"); err != nil {
		t.Fatal(err)
	}
	if _, region := echoedConfig(t, s); region != "eu" {
		t.Fatalf("overridden region = %q, want eu", region)
	}
}

func TestShopApiKeyPrecedence(t *testing.T) {
	// A stored api_key override wins over the settings/baked chain key; the chain
	// key (baked build default) only fills in when no config value is stored.
	s, cs := cfgShop(t, func(string) string { return "chainkey" })
	_ = cs.Set("cfgshop", "api_key", "storedkey")
	if key, _ := echoedConfig(t, s); key != "storedkey" {
		t.Fatalf("api_key = %q, want storedkey (stored config wins)", key)
	}

	// With no stored key, the chain key fills in.
	s3, _ := cfgShop(t, func(string) string { return "chainkey" })
	if key, _ := echoedConfig(t, s3); key != "chainkey" {
		t.Fatalf("api_key = %q, want chainkey (baked fallback)", key)
	}

	// With no chain key, a stored api_key fills in and makes the shop Ready.
	s2, cs2 := cfgShop(t, func(string) string { return "" })
	if s2.Ready() {
		t.Fatal("shop must not be ready before a key is configured")
	}
	_ = cs2.Set("cfgshop", "api_key", "storedkey")
	if !s2.Ready() {
		t.Fatal("stored api_key must make the shop ready")
	}
	if key, _ := echoedConfig(t, s2); key != "storedkey" {
		t.Fatalf("api_key = %q, want storedkey", key)
	}
}

func TestShopNotReadyWithoutKey(t *testing.T) {
	s, _ := cfgShop(t, func(string) string { return "" })
	if s.Ready() {
		t.Fatal("keyless needs_key shop must not be ready")
	}
	if _, err := s.Search(context.Background(), ShopQuery{Query: "q", Limit: 10}); !errors.Is(err, ErrShopKeyMissing) {
		t.Fatalf("search err = %v, want ErrShopKeyMissing", err)
	}
}

// cfgProvSrc echoes its config flag into the launch spec args.
const cfgProvSrc = `
meta = { id = "cfgprov", name = "Cfg Prov", mod_layout = "none",
  config = { { key = "flag", type = "string", name = "Flag", default = "def" } } }
function versions() return { "1.0" } end
function install(ctx)
  return { java_major = 8, args = { ctx.config.flag or "nil" } }
end
`

func TestProviderConfigInjection(t *testing.T) {
	cs := NewConfigStore(filepath.Join(t.TempDir(), "provider-config.json"))
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	r.SetConfigStore(cs)
	e, err := r.AddSource(context.Background(), cfgProvSrc, true)
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	spec, err := e.Provider.Install(context.Background(), t.TempDir(), "1.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Args) != 1 || spec.Args[0] != "def" {
		t.Fatalf("default args = %v, want [def]", spec.Args)
	}

	// A stored override is picked up live (configFn resolves per call).
	if err := cs.Set("cfgprov", "flag", "xyz"); err != nil {
		t.Fatal(err)
	}
	spec, err = e.Provider.Install(context.Background(), t.TempDir(), "1.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Args[0] != "xyz" {
		t.Fatalf("overridden args = %v, want [xyz]", spec.Args)
	}
}
