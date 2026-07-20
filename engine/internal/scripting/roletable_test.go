package scripting

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

// dualRoleSource is a minimal multi-role script: a shop role (mod+modpack) and a
// hidden provider role, sharing one meta.id and one config option, and answering
// to the alias "acme_modpacks".
const dualRoleSource = `
meta = {
  id = "acme",
  name = "Acme",
  version = "1.0.0",
  author = "t",
  aliases = { "acme_modpacks" },
  config = { { key = "api_key", type = "secret", name = "Key" } },
  permissions = { { kind = "network", reason = "r" } },
}
shop = {
  kinds = { "mod", "modpack" },
  needs_key = true,
  home = function(ctx) return { sections = {} } end,
  search = function(ctx) return { projects = {}, total = 0 } end,
  detail = function(ctx) return { project = {}, body = "", body_format = "markdown" } end,
  versions = function(ctx) return { versions = {} } end,
  resolve_file = function(ctx) return { url = "http://x", filename = "x" } end,
}
provider = {
  hidden = true,
  mod_layout = "mods",
  versions = function() return {} end,
  install = function(ctx) return { java_major = 17, args = { ctx.config.api_key or "" } } end,
}
`

// loadDualRole registers dualRoleSource into a fresh registry + shop set backed
// by a shared config store, returning all three.
func loadDualRole(t *testing.T, cfg *ConfigStore) (*Registry, *ShopSet) {
	t.Helper()
	host := NewHost(nil, nil, nil)
	reg := NewRegistry(host, nil)
	reg.SetConfigStore(cfg)
	ss := NewShopSet(host, nil, nil)
	ss.SetConfigStore(cfg)
	if _, err := ss.AddSource(context.Background(), dualRoleSource, true); err != nil {
		t.Fatalf("shop AddSource: %v", err)
	}
	if _, err := reg.AddSourceWithConfig(context.Background(), dualRoleSource, true, cfg); err != nil {
		t.Fatalf("provider AddSourceWithConfig: %v", err)
	}
	return reg, ss
}

// TestRoleTableRegistersBothRoles proves a single source registers into both
// subsystems with role-scoped meta applied.
func TestRoleTableRegistersBothRoles(t *testing.T) {
	reg, ss := loadDualRole(t, NewConfigStore(filepath.Join(t.TempDir(), "c.json")))

	sh, ok := ss.Get("acme")
	if !ok {
		t.Fatal("shop role not registered")
	}
	if !sh.Meta().NeedsKey {
		t.Error("shop role should read needs_key from the shop table")
	}
	if len(sh.Meta().Kinds) != 2 {
		t.Errorf("shop kinds = %v, want mod+modpack", sh.Meta().Kinds)
	}

	e, ok := reg.Get("acme")
	if !ok {
		t.Fatal("provider role not registered")
	}
	if !e.Meta.Hidden {
		t.Error("provider role should read hidden from the provider table")
	}
	if e.Meta.ModLayout != "mods" {
		t.Errorf("provider mod_layout = %q, want mods", e.Meta.ModLayout)
	}
}

// TestRoleTableAliasResolves proves an old id resolves to the canonical entry in
// both subsystems and is never listed as a separate entry.
func TestRoleTableAliasResolves(t *testing.T) {
	reg, ss := loadDualRole(t, NewConfigStore(filepath.Join(t.TempDir(), "c.json")))

	sh, ok := ss.Get("acme_modpacks")
	if !ok || sh.Meta().ID != "acme" {
		t.Fatalf("shop alias did not resolve to acme: ok=%v", ok)
	}
	e, ok := reg.Get("acme_modpacks")
	if !ok || e.Meta.ID != "acme" {
		t.Fatalf("provider alias did not resolve to acme: ok=%v", ok)
	}

	if got := len(reg.List()); got != 1 {
		t.Errorf("registry List has %d entries, want 1 (no alias dupe)", got)
	}
	if got := len(ss.List()); got != 1 {
		t.Errorf("shop List has %d entries, want 1 (no alias dupe)", got)
	}
}

func TestRegistryRejectsCanonicalAliasCollisionsWithoutChangingResolution(t *testing.T) {
	host := NewHost(nil, nil, nil)
	reg := NewRegistry(host, nil)
	if _, err := reg.AddSource(context.Background(), dualRoleSource, true); err != nil {
		t.Fatalf("register built-in: %v", err)
	}

	tests := []struct {
		name    string
		id      string
		aliases string
	}{
		{name: "canonical claims existing alias", id: "acme_modpacks"},
		{name: "alias claims existing canonical", id: "other", aliases: `aliases = { "acme" },`},
		{name: "alias claims existing alias", id: "other", aliases: `aliases = { "acme_modpacks" },`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := fmt.Sprintf(`
meta = { id = %q, name = "Other", version = "1", author = "t", %s }
function versions() return {} end
function install(ctx) return { java_major = 17, args = {} } end
`, tt.id, tt.aliases)
			if _, err := reg.AddSource(context.Background(), source, false); !errors.Is(err, ErrProviderIDConflict) {
				t.Fatalf("AddSource error = %v, want ErrProviderIDConflict", err)
			}
			assertRegistryAliasResolution(t, reg)
		})
	}
}

func TestShopSetRejectsCanonicalAliasCollisionsWithoutChangingResolution(t *testing.T) {
	host := NewHost(nil, nil, nil)
	shops := NewShopSet(host, nil, nil)
	if _, err := shops.AddSource(context.Background(), dualRoleSource, true); err != nil {
		t.Fatalf("register built-in: %v", err)
	}

	tests := []struct {
		name    string
		id      string
		aliases string
	}{
		{name: "canonical claims existing alias", id: "acme_modpacks"},
		{name: "alias claims existing canonical", id: "other", aliases: `aliases = { "acme" },`},
		{name: "alias claims existing alias", id: "other", aliases: `aliases = { "acme_modpacks" },`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := fmt.Sprintf(`
meta = { id = %q, name = "Other", version = "1", author = "t", %s }
function home(ctx) return { sections = {} } end
function search(ctx) return { projects = {}, total = 0 } end
function detail(ctx) return { project = {}, body = "", body_format = "markdown" } end
function versions(ctx) return { versions = {} } end
function resolve_file(ctx) return { url = "http://x", filename = "x" } end
`, tt.id, tt.aliases)
			if _, err := shops.AddSource(context.Background(), source, false); !errors.Is(err, ErrProviderIDConflict) {
				t.Fatalf("AddSource error = %v, want ErrProviderIDConflict", err)
			}
			assertShopAliasResolution(t, shops)
		})
	}
}

func assertRegistryAliasResolution(t *testing.T, reg *Registry) {
	t.Helper()
	for _, id := range []string{"acme", "acme_modpacks"} {
		e, ok := reg.Get(id)
		if !ok || e.Meta.ID != "acme" {
			t.Fatalf("provider %q resolution changed: ok=%v entry=%v", id, ok, e)
		}
	}
	if got := len(reg.List()); got != 1 {
		t.Fatalf("registry contains %d entries after rejected collision, want 1", got)
	}
}

func assertShopAliasResolution(t *testing.T, shops *ShopSet) {
	t.Helper()
	for _, id := range []string{"acme", "acme_modpacks"} {
		shop, ok := shops.Get(id)
		if !ok || shop.Meta().ID != "acme" {
			t.Fatalf("shop %q resolution changed: ok=%v shop=%v", id, ok, shop)
		}
	}
	if got := len(shops.List()); got != 1 {
		t.Fatalf("shop set contains %d entries after rejected collision, want 1", got)
	}
}

// TestRoleTableSharedConfig proves both roles read one stored config entry keyed
// by the canonical id.
func TestRoleTableSharedConfig(t *testing.T) {
	cfg := NewConfigStore(filepath.Join(t.TempDir(), "c.json"))
	reg, ss := loadDualRole(t, cfg)
	if err := cfg.Set("acme", "api_key", "shared-secret"); err != nil {
		t.Fatal(err)
	}

	sh, _ := ss.Get("acme")
	if got := sh.effectiveKey(); got != "shared-secret" {
		t.Errorf("shop effectiveKey = %q, want shared-secret", got)
	}

	e, _ := reg.Get("acme")
	spec, err := e.Provider.Install(context.Background(), t.TempDir(), "1/2", nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(spec.Args) != 1 || spec.Args[0] != "shared-secret" {
		t.Errorf("provider ctx.config.api_key = %v, want [shared-secret]", spec.Args)
	}
}

// TestLegacySingleRoleStillLoads proves a legacy top-level-function script (no
// role tables) still registers and reads meta-level fields.
func TestLegacySingleRoleStillLoads(t *testing.T) {
	const legacy = `
meta = { id = "legacy", name = "L", version = "1", author = "t", hidden = true, mod_layout = "plugins",
  permissions = { { kind = "network", reason = "r" } } }
function versions() return {} end
function install(ctx) return { java_major = 17, args = {} } end
`
	reg := NewRegistry(NewHost(nil, nil, nil), nil)
	e, err := reg.AddSource(context.Background(), legacy, true)
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if !e.Meta.Hidden || e.Meta.ModLayout != "plugins" {
		t.Errorf("legacy meta-level fields lost: hidden=%v layout=%q", e.Meta.Hidden, e.Meta.ModLayout)
	}
	if _, err := e.Provider.Install(context.Background(), t.TempDir(), "v", nil); err != nil {
		t.Fatalf("legacy install: %v", err)
	}
}

// TestSourceRolesDetectsTables checks role detection for the merged sources.
func TestSourceRolesDetectsTables(t *testing.T) {
	host := NewHost(nil, nil, nil)
	shop, provider, err := sourceRoles(context.Background(), host, dualRoleSource)
	if err != nil {
		t.Fatal(err)
	}
	if !shop || !provider {
		t.Fatalf("dual role detection: shop=%v provider=%v", shop, provider)
	}
}

func TestLoadBuiltinSourcesReturnsOnlyActualMultiRoleSourceIDs(t *testing.T) {
	host := NewHost(nil, nil, nil)
	reg := NewRegistry(host, nil)
	shops := NewShopSet(host, nil, nil)

	ids, err := LoadBuiltinSources(context.Background(), reg, shops, nil)
	if err != nil {
		t.Fatalf("LoadBuiltinSources: %v", err)
	}
	for _, id := range []string{"curseforge", "ftb", "modrinth"} {
		if !ids.Contains(id) {
			t.Errorf("multi-role source ids do not contain %q", id)
		}
	}
	if ids.Contains("curseforge_modpacks") {
		t.Error("alias curseforge_modpacks must not be tracked as a canonical multi-role source id")
	}
	const claimedBuiltinAlias = `
meta = { id = "curseforge_modpacks", name = "Claim", version = "1", author = "t" }
function home(ctx) return { sections = {} } end
function search(ctx) return { projects = {}, total = 0 } end
function detail(ctx) return { project = {}, body = "", body_format = "markdown" } end
function versions(ctx) return {} end
function resolve_file(ctx) return { url = "http://x", filename = "x" } end
function install(ctx) return { java_major = 17, args = {} } end
`
	if _, err := reg.AddSource(context.Background(), claimedBuiltinAlias, false); !errors.Is(err, ErrProviderIDConflict) {
		t.Fatalf("provider claiming built-in alias error = %v, want ErrProviderIDConflict", err)
	}
	if _, err := shops.AddSource(context.Background(), claimedBuiltinAlias, false); !errors.Is(err, ErrProviderIDConflict) {
		t.Fatalf("shop claiming built-in alias error = %v, want ErrProviderIDConflict", err)
	}
	if entry, ok := reg.Get("curseforge_modpacks"); !ok || entry.Meta.ID != "curseforge" {
		t.Fatalf("provider built-in alias resolution changed after rejection: ok=%v entry=%v", ok, entry)
	}
	if shop, ok := shops.Get("curseforge_modpacks"); !ok || shop.Meta().ID != "curseforge" {
		t.Fatalf("shop built-in alias resolution changed after rejection: ok=%v shop=%v", ok, shop)
	}

	// Matching ids in the two registries are not enough: only one script that
	// actually declared both roles is eligible for shared config routing.
	const unrelatedCollision = `
meta = { id = "coincidental", name = "Coincidental", version = "1", author = "t" }
function home(ctx) return { sections = {} } end
function search(ctx) return { projects = {}, total = 0 } end
function detail(ctx) return { project = {}, body = "", body_format = "markdown" } end
function versions(ctx) return {} end
function resolve_file(ctx) return { url = "http://x", filename = "x" } end
function install(ctx) return { java_major = 17, args = {} } end
`
	if _, err := reg.AddSource(context.Background(), unrelatedCollision, false); err != nil {
		t.Fatalf("register unrelated provider: %v", err)
	}
	if _, err := shops.AddSource(context.Background(), unrelatedCollision, false); err != nil {
		t.Fatalf("register unrelated shop: %v", err)
	}
	if _, ok := reg.Get("coincidental"); !ok {
		t.Fatal("unrelated provider was not registered")
	}
	if _, ok := shops.Get("coincidental"); !ok {
		t.Fatal("unrelated shop was not registered")
	}
	if ids.Contains("coincidental") {
		t.Error("unrelated provider/shop id collision must not be treated as a shared-config source")
	}
}

// TestParseMetaAliases covers alias validation.
func TestParseMetaAliases(t *testing.T) {
	m, err := metaFrom(t, `meta = { id = "x", aliases = { "x_old", "x_older" } }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Aliases) != 2 || m.Aliases[0] != "x_old" {
		t.Fatalf("aliases = %v", m.Aliases)
	}

	bad := map[string]string{
		"dup with id":  `meta = { id = "x", aliases = { "x" } }`,
		"dup alias":    `meta = { id = "x", aliases = { "y", "y" } }`,
		"invalid char": `meta = { id = "x", aliases = { "a.b" } }`,
		"non-string":   `meta = { id = "x", aliases = { 5 } }`,
	}
	for name, src := range bad {
		if _, err := metaFrom(t, src); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
