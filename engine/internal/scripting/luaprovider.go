package scripting

import (
	"context"
	"fmt"

	"github.com/000hen/justhostmc/engine/internal/provider"
)

// LuaProvider adapts a Lua script to the provider.Provider interface: Versions
// and Install run the script's versions()/install() in a fresh sandbox each
// call, with the script's currently-granted permissions enforced.
type LuaProvider struct {
	meta     Meta
	source   string
	host     *Host
	builtin  bool
	assetDir string // dir with files bundled alongside the script (custom jar), if any
	grantsFn func() GrantSet
	configFn func() map[string]string
}

// newLuaProvider parses a script's meta (in a throwaway sandbox) and returns the
// adapter. grantsFn is set by the registry once the id is known.
func newLuaProvider(ctx context.Context, host *Host, source string, builtin bool, assetDir string) (*LuaProvider, error) {
	inv := &invocation{ctx: ctx, host: host}
	L, err := inv.prepare(source)
	if err != nil {
		return nil, err
	}
	defer L.Close()
	meta, err := parseMeta(L)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScriptInvalid, err)
	}
	return &LuaProvider{meta: meta, source: source, host: host, builtin: builtin, assetDir: assetDir}, nil
}

// Meta returns the script's declared metadata.
func (p *LuaProvider) Meta() Meta { return p.meta }

func (p *LuaProvider) grants() GrantSet {
	if p.grantsFn != nil {
		return p.grantsFn()
	}
	return nil
}

// config resolves the values the script sees as ctx.config / jhmc.config:
// declared defaults merged with stored overrides. Returns nil (no config) when
// the script declares none and none is stored.
func (p *LuaProvider) config() map[string]string {
	var stored map[string]string
	if p.configFn != nil {
		stored = p.configFn()
	}
	if len(p.meta.Config) == 0 && len(stored) == 0 {
		return nil
	}
	return EffectiveConfig(p.meta.Config, stored)
}

// Versions implements provider.Provider.
func (p *LuaProvider) Versions(ctx context.Context) ([]string, error) {
	inv := &invocation{ctx: ctx, host: p.host, granted: p.grants(), assetDir: p.assetDir, config: p.config()}
	return inv.versions(p.source)
}

// Install implements provider.Provider.
func (p *LuaProvider) Install(ctx context.Context, dir, version string, progress func(provider.Progress)) (provider.LaunchSpec, error) {
	inv := &invocation{ctx: ctx, host: p.host, granted: p.grants(), report: progress, assetDir: p.assetDir, config: p.config()}
	return inv.install(p.source, dir, version)
}

// Update implements provider.Updater; scripts without an update() function
// yield provider.ErrUpdateUnsupported.
func (p *LuaProvider) Update(ctx context.Context, dir, version, oldVersion string, progress func(provider.Progress)) (provider.LaunchSpec, error) {
	inv := &invocation{ctx: ctx, host: p.host, granted: p.grants(), report: progress, assetDir: p.assetDir, config: p.config()}
	return inv.update(p.source, dir, version, oldVersion)
}
