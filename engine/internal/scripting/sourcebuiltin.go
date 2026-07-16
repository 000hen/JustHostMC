package scripting

import (
	"context"
	"embed"
	"fmt"
	"strings"
)

// Built-in "source" scripts declare more than one role in a single file: a
// `shop` role table (browse/search) and a hidden `provider` role table (install
// a whole modpack as a server), sharing one `meta` (id, config, permissions).
// They live in their own embedded dir so neither the provider loader nor the
// shop loader picks them up on its own.
//
//go:embed builtin_sources/*.lua
var builtinSourcesFS embed.FS

// SourceIDs is the set of canonical ids loaded from scripts that actually
// declare both a shop role and a provider role. It deliberately excludes
// aliases and unrelated entries that happen to use the same id in both
// registries.
type SourceIDs struct {
	ids map[string]struct{}
}

// Contains reports whether id is the canonical id of a loaded multi-role
// source script.
func (s SourceIDs) Contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}

// LoadBuiltinSources registers every embedded multi-role source script: its
// shop role into shops and its provider role into providers. Both roles keep the
// script's canonical meta.id, so one stored config entry (keyed by that id in
// the shop config store) is shared. A script that declares no `shop`/`provider`
// role table still registers into whichever subsystem it defines functions for.
//
// providerCfg is the config reader handed to the provider role so its ctx.config
// resolves from the shared shop config store (plus any baked-key fallbacks the
// caller layers on) rather than the provider registry's own store.
func LoadBuiltinSources(ctx context.Context, providers *Registry, shops *ShopSet, providerCfg ConfigReader) (SourceIDs, error) {
	sourceIDs := SourceIDs{ids: make(map[string]struct{})}
	entries, err := builtinSourcesFS.ReadDir("builtin_sources")
	if err != nil {
		return SourceIDs{}, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		src, err := builtinSourcesFS.ReadFile("builtin_sources/" + e.Name())
		if err != nil {
			return SourceIDs{}, fmt.Errorf("builtin source %s: %w", e.Name(), err)
		}
		hasShop, hasProvider, err := sourceRoles(ctx, providers.host, string(src))
		if err != nil {
			return SourceIDs{}, fmt.Errorf("builtin source %s: %w", e.Name(), err)
		}
		var sourceID string
		if hasShop {
			shop, err := shops.AddSource(ctx, string(src), true)
			if err != nil {
				return SourceIDs{}, fmt.Errorf("builtin source %s (shop): %w", e.Name(), err)
			}
			sourceID = shop.Meta().ID
		}
		if hasProvider {
			entry, err := providers.AddSourceWithConfig(ctx, string(src), true, providerCfg)
			if err != nil {
				return SourceIDs{}, fmt.Errorf("builtin source %s (provider): %w", e.Name(), err)
			}
			sourceID = entry.Meta.ID
		}
		if hasShop && hasProvider {
			sourceIDs.ids[sourceID] = struct{}{}
		}
	}
	return sourceIDs, nil
}

// sourceRoles reports which role tables a source script declares. A script with
// neither table (a legacy single-role script placed here) is treated as both so
// it still registers where its top-level functions apply; callers only place
// dual-role scripts in builtin_sources, so in practice both are true.
func sourceRoles(ctx context.Context, host *Host, source string) (shop, provider bool, err error) {
	inv := &invocation{ctx: ctx, host: host}
	L, err := inv.prepare(source)
	if err != nil {
		return false, false, err
	}
	defer L.Close()
	shop = roleTable(L, "shop") != nil
	provider = roleTable(L, "provider") != nil
	if !shop && !provider {
		return true, true, nil
	}
	return shop, provider, nil
}
