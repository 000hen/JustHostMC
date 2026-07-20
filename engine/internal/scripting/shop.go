package scripting

import (
	"context"
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// The shop subsystem runs Lua "shop" scripts that browse/search online mod
// sources (Modrinth, CurseForge, ...). A shop script declares the usual
// `meta` table (plus `needs_key = true` when the source wants an API key)
// and these global functions, each taking one ctx table and returning one
// table:
//
//	home(ctx{mc_version, loader, kind, config})            -> {sections={{title_key=, projects={...}}, ...}}
//	search(ctx{query, mc_version, loader, kind, sort,
//	           offset, limit, config})                      -> {projects={...}, total=}
//	detail(ctx{project_id, config})                         -> project detail table
//	versions(ctx{project_id, mc_version, loader, config})   -> {versions={...}}
//	resolve_file(ctx{project_id, version_id, mc_version,
//	             loader, config})                           -> {url=, filename=, size=, sha1=|sha512=}
//
// resolve_file with an empty version_id must pick the latest version
// compatible with mc_version/loader (used for dependency installs).

// ShopProject is one browse/search result card.
type ShopProject struct {
	ID           string
	Slug         string
	Title        string
	Summary      string
	IconURL      string
	Author       string
	Downloads    int64
	Follows      int64
	Categories   []string
	ProjectType  string
	Distribution ShopDistribution
}

// ShopDistribution describes whether a project may be downloaded through the
// shop API or must be obtained from the source website.
type ShopDistribution string

const (
	ShopDistributionUnknown     ShopDistribution = ""
	ShopDistributionDirect      ShopDistribution = "direct"
	ShopDistributionWebsiteOnly ShopDistribution = "website_only"
)

// ShopCategory is one source-native search filter exposed by a shop script.
type ShopCategory struct {
	ID              string
	Name            string
	Slug            string
	LocalizationKey string
}

// ShopPage is a paged project list.
type ShopPage struct {
	Projects []ShopProject
	Total    int64
	Offset   int32
}

// ShopSection is one landing-page row ("Popular", "Recently updated", ...).
// TitleKey is an i18n key resolved by the frontend.
type ShopSection struct {
	TitleKey string
	Projects []ShopProject
}

// ShopGalleryImage is one screenshot in a project's gallery.
type ShopGalleryImage struct {
	URL         string
	Title       string
	Description string
}

// ShopLinks are a project's outbound links (all optional).
type ShopLinks struct {
	Website string
	Source  string
	Issues  string
	Wiki    string
	Discord string
}

// ShopDetail is the full project page.
type ShopDetail struct {
	Project      ShopProject
	Body         string
	BodyFormat   string // "markdown" | "html"
	Gallery      []ShopGalleryImage
	Links        ShopLinks
	GameVersions []string
	Loaders      []string
	License      string
	Updated      string // RFC3339 UTC
}

// ShopDependency is a relationship a version declares on another project.
type ShopDependency struct {
	ProjectID string
	Title     string
	Required  bool
}

// ShopVersion is one installable version of a project.
type ShopVersion struct {
	ID            string
	Name          string
	VersionNumber string
	Channel       string // "release" | "beta" | "alpha"
	GameVersions  []string
	Loaders       []string
	Date          string // RFC3339 UTC
	Downloads     int64
	Filename      string
	SizeBytes     int64
	Dependencies  []ShopDependency
}

// ShopFile is a concrete downloadable artifact resolved for install.
type ShopFile struct {
	URL      string
	Filename string
	Size     int64
	SHA1     string
	SHA512   string
}

// ShopQuery carries the browse/search filter set into a script.
type ShopQuery struct {
	Query      string
	MCVersion  string   // empty = no version filter
	Loader     string   // empty = no loader filter
	Kind       string   // "mod" | "plugin"
	Categories []string // source-native category ids/slugs; empty = all
	Sort       string   // "relevance" | "downloads" | "follows" | "newest" | "updated"
	Offset     int
	Limit      int
}

// shopFuncs are the globals every shop script must define.
var shopFuncs = []string{"home", "search", "detail", "versions", "resolve_file"}

// LuaShop adapts one sandboxed Lua shop script to the engine. It is the shop
// analog of LuaParser.
type LuaShop struct {
	meta     Meta
	source   string
	host     *Host
	builtin  bool
	grantsFn func() GrantSet
	keyFn    func() string // resolves the shop's API key ("" = none)
	configFn func() map[string]string
}

// newLuaShop compiles source in a throwaway sandbox and validates its meta
// and required globals.
func newLuaShop(ctx context.Context, host *Host, source string, builtin bool) (*LuaShop, error) {
	inv := &invocation{ctx: ctx, host: host}
	L, err := inv.prepare(source)
	if err != nil {
		return nil, err
	}
	defer L.Close()
	meta, err := parseShopMeta(L)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScriptInvalid, err)
	}
	for _, fn := range shopFuncs {
		if shopFn(L, fn).Type() != lua.LTFunction {
			return nil, fmt.Errorf("%w: script does not define %s(ctx)", ErrScriptInvalid, fn)
		}
	}
	return &LuaShop{meta: meta, source: source, host: host, builtin: builtin}, nil
}

// shopFn resolves a shop entry point, preferring a function in the global `shop`
// role table and falling back to a top-level global (the legacy layout).
func shopFn(L *lua.LState, name string) lua.LValue {
	if tbl := roleTable(L, "shop"); tbl != nil {
		if f := tbl.RawGetString(name); f.Type() == lua.LTFunction {
			return f
		}
	}
	return L.GetGlobal(name)
}

// Meta returns the shop's declared metadata.
func (s *LuaShop) Meta() Meta { return s.meta }

// Builtin reports whether this is a first-party shop.
func (s *LuaShop) Builtin() bool { return s.builtin }

// Key resolves the shop's API key; "" when none is configured.
func (s *LuaShop) Key() string {
	if s.keyFn != nil {
		return s.keyFn()
	}
	return ""
}

// Ready reports whether the shop can serve requests (a key exists when the
// script declares needs_key). The key may come from the settings/baked chain or
// a stored api_key config override.
func (s *LuaShop) Ready() bool { return !s.meta.NeedsKey || s.effectiveKey() != "" }

// storedConfig returns the shop's stored typed-config overrides (nil when none).
func (s *LuaShop) storedConfig() map[string]string {
	if s.configFn != nil {
		return s.configFn()
	}
	return nil
}

// effectiveKey resolves the shop's API key: a stored `api_key` config override
// wins, and the keyFn (baked build default) only fills in when config is empty.
func (s *LuaShop) effectiveKey() string {
	if k := s.storedConfig()["api_key"]; k != "" {
		return k
	}
	return s.Key()
}

func (s *LuaShop) grants() GrantSet {
	if s.grantsFn != nil {
		return s.grantsFn()
	}
	return nil
}

// call runs one script global with a ctx table built from fields, returning
// the result table. It enforces the needs_key gate first.
func (s *LuaShop) call(ctx context.Context, fn string, fields map[string]lua.LValue, slices ...map[string][]string) (*lua.LTable, error) {
	tbl, _, err := s.callFunction(ctx, fn, false, fields, slices...)
	return tbl, err
}

func (s *LuaShop) callOptional(ctx context.Context, fn string, fields map[string]lua.LValue, slices ...map[string][]string) (*lua.LTable, bool, error) {
	return s.callFunction(ctx, fn, true, fields, slices...)
}

func (s *LuaShop) callFunction(ctx context.Context, fn string, optional bool, fields map[string]lua.LValue, slices ...map[string][]string) (*lua.LTable, bool, error) {
	if !s.Ready() {
		return nil, false, fmt.Errorf("%w: shop %s", ErrShopKeyMissing, s.meta.ID)
	}
	inv := &invocation{ctx: ctx, host: s.host, granted: s.grants()}
	L, err := inv.prepare(s.source)
	if err != nil {
		return nil, false, err
	}
	defer L.Close()

	f := shopFn(L, fn)
	if f.Type() != lua.LTFunction {
		if optional {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%w: script does not define %s(ctx)", ErrScriptInvalid, fn)
	}
	ctxTbl := L.NewTable()
	for k, v := range fields {
		ctxTbl.RawSetString(k, v)
	}
	if len(slices) > 0 {
		for key, values := range slices[0] {
			items := L.NewTable()
			for _, value := range values {
				items.Append(lua.LString(value))
			}
			ctxTbl.RawSetString(key, items)
		}
	}
	cfg := L.NewTable()
	for k, v := range EffectiveConfig(s.meta.Config, s.storedConfig()) {
		cfg.RawSetString(k, lua.LString(v))
	}
	// A stored api_key config wins for the reserved api_key; the keyFn (baked
	// build default) only fills in when no stored value is present.
	if _, ok := s.storedConfig()["api_key"]; !ok {
		if key := s.Key(); key != "" {
			cfg.RawSetString("api_key", lua.LString(key))
		}
	}
	ctxTbl.RawSetString("config", cfg)

	if err := L.CallByParam(lua.P{Fn: f, NRet: 1, Protect: true}, ctxTbl); err != nil {
		return nil, true, s.bridgeErr(inv.mapErr(err))
	}
	ret := L.Get(-1)
	L.Pop(1)
	tbl, ok := ret.(*lua.LTable)
	if !ok {
		return nil, true, fmt.Errorf("%w: %s(ctx) did not return a table", ErrScriptInvalid, fn)
	}
	return tbl, true, nil
}

// bridgeErr maps well-known error phrases raised by shop scripts (via
// error("...")) onto the engine's sentinel errors, mirroring how provider
// scripts bridge "version not found".
func (s *LuaShop) bridgeErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not distributable"):
		return fmt.Errorf("%w: %v", ErrShopNotDistributable, err)
	case strings.Contains(msg, "not found"):
		return fmt.Errorf("%w: %v", ErrShopNotFound, err)
	}
	return err
}

// Categories returns source-native search filters. The Lua function is
// optional so existing third-party shop scripts remain compatible.
func (s *LuaShop) Categories(ctx context.Context, kind string) ([]ShopCategory, error) {
	tbl, found, err := s.callOptional(ctx, "categories", map[string]lua.LValue{
		"kind": lua.LString(kind),
	})
	if err != nil || !found {
		return nil, err
	}
	items, _ := tbl.RawGetString("categories").(*lua.LTable)
	if items == nil {
		return nil, nil
	}
	out := make([]ShopCategory, 0, items.Len())
	items.ForEach(func(_, value lua.LValue) {
		if item, ok := value.(*lua.LTable); ok {
			out = append(out, ShopCategory{
				ID:              strField(item, "id"),
				Name:            strField(item, "name"),
				Slug:            strField(item, "slug"),
				LocalizationKey: strField(item, "localization_key"),
			})
		}
	})
	return out, nil
}

// Home returns the landing-page sections for the given pre-filter.
func (s *LuaShop) Home(ctx context.Context, q ShopQuery) ([]ShopSection, error) {
	tbl, err := s.call(ctx, "home", map[string]lua.LValue{
		"mc_version": lua.LString(q.MCVersion),
		"loader":     lua.LString(q.Loader),
		"kind":       lua.LString(q.Kind),
	})
	if err != nil {
		return nil, err
	}
	var sections []ShopSection
	if sv, ok := tbl.RawGetString("sections").(*lua.LTable); ok {
		sv.ForEach(func(_, v lua.LValue) {
			st, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			sections = append(sections, ShopSection{
				TitleKey: strField(st, "title_key"),
				Projects: readProjects(st),
			})
		})
	}
	return sections, nil
}

// Search runs a paged project search.
func (s *LuaShop) Search(ctx context.Context, q ShopQuery) (ShopPage, error) {
	tbl, err := s.call(ctx, "search", map[string]lua.LValue{
		"query":      lua.LString(q.Query),
		"mc_version": lua.LString(q.MCVersion),
		"loader":     lua.LString(q.Loader),
		"kind":       lua.LString(q.Kind),
		"sort":       lua.LString(q.Sort),
		"offset":     lua.LNumber(q.Offset),
		"limit":      lua.LNumber(q.Limit),
	}, map[string][]string{"categories": q.Categories})
	if err != nil {
		return ShopPage{}, err
	}
	return ShopPage{
		Projects: readProjects(tbl),
		Total:    intField(tbl, "total"),
		Offset:   int32(q.Offset),
	}, nil
}

// Detail returns the full project page.
func (s *LuaShop) Detail(ctx context.Context, projectID string) (ShopDetail, error) {
	tbl, err := s.call(ctx, "detail", map[string]lua.LValue{
		"project_id": lua.LString(projectID),
	})
	if err != nil {
		return ShopDetail{}, err
	}
	d := ShopDetail{
		Project:      readProject(tbl),
		Body:         strField(tbl, "body"),
		BodyFormat:   strings.ToLower(strField(tbl, "body_format")),
		GameVersions: strSlice(tbl, "game_versions"),
		Loaders:      strSlice(tbl, "loaders"),
		License:      strField(tbl, "license"),
		Updated:      strField(tbl, "updated"),
	}
	if g, ok := tbl.RawGetString("gallery").(*lua.LTable); ok {
		g.ForEach(func(_, v lua.LValue) {
			if it, ok := v.(*lua.LTable); ok {
				d.Gallery = append(d.Gallery, ShopGalleryImage{
					URL:         strField(it, "url"),
					Title:       strField(it, "title"),
					Description: strField(it, "description"),
				})
			}
		})
	}
	if l, ok := tbl.RawGetString("links").(*lua.LTable); ok {
		d.Links = ShopLinks{
			Website: strField(l, "website"),
			Source:  strField(l, "source"),
			Issues:  strField(l, "issues"),
			Wiki:    strField(l, "wiki"),
			Discord: strField(l, "discord"),
		}
	}
	return d, nil
}

// Versions lists installable versions filtered to mc_version/loader.
func (s *LuaShop) Versions(ctx context.Context, projectID, mcVersion, loader string) ([]ShopVersion, error) {
	tbl, err := s.call(ctx, "versions", map[string]lua.LValue{
		"project_id": lua.LString(projectID),
		"mc_version": lua.LString(mcVersion),
		"loader":     lua.LString(loader),
	})
	if err != nil {
		return nil, err
	}
	var out []ShopVersion
	if vs, ok := tbl.RawGetString("versions").(*lua.LTable); ok {
		vs.ForEach(func(_, v lua.LValue) {
			vt, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			ver := ShopVersion{
				ID:            strField(vt, "id"),
				Name:          strField(vt, "name"),
				VersionNumber: strField(vt, "version_number"),
				Channel:       strings.ToLower(strField(vt, "channel")),
				GameVersions:  strSlice(vt, "game_versions"),
				Loaders:       strSlice(vt, "loaders"),
				Date:          strField(vt, "date"),
				Downloads:     intField(vt, "downloads"),
				Filename:      strField(vt, "filename"),
				SizeBytes:     intField(vt, "size"),
			}
			if deps, ok := vt.RawGetString("dependencies").(*lua.LTable); ok {
				deps.ForEach(func(_, dv lua.LValue) {
					if dt, ok := dv.(*lua.LTable); ok {
						ver.Dependencies = append(ver.Dependencies, ShopDependency{
							ProjectID: strField(dt, "project_id"),
							Title:     strField(dt, "title"),
							Required:  boolField(dt, "required"),
						})
					}
				})
			}
			out = append(out, ver)
		})
	}
	return out, nil
}

// ResolveFile turns (project, version) into a concrete downloadable artifact.
// versionID "" means "latest compatible with mcVersion/loader".
func (s *LuaShop) ResolveFile(ctx context.Context, projectID, versionID, mcVersion, loader string) (ShopFile, error) {
	tbl, err := s.call(ctx, "resolve_file", map[string]lua.LValue{
		"project_id": lua.LString(projectID),
		"version_id": lua.LString(versionID),
		"mc_version": lua.LString(mcVersion),
		"loader":     lua.LString(loader),
	})
	if err != nil {
		return ShopFile{}, err
	}
	f := ShopFile{
		URL:      strField(tbl, "url"),
		Filename: strField(tbl, "filename"),
		Size:     intField(tbl, "size"),
		SHA1:     strField(tbl, "sha1"),
		SHA512:   strField(tbl, "sha512"),
	}
	if f.URL == "" {
		return ShopFile{}, fmt.Errorf("%w: %s", ErrShopNotDistributable, projectID)
	}
	if f.Filename == "" {
		return ShopFile{}, fmt.Errorf("%w: resolve_file returned no filename", ErrScriptInvalid)
	}
	return f, nil
}

// readProjects reads tbl.projects into a slice.
func readProjects(tbl *lua.LTable) []ShopProject {
	var out []ShopProject
	if pv, ok := tbl.RawGetString("projects").(*lua.LTable); ok {
		pv.ForEach(func(_, v lua.LValue) {
			if pt, ok := v.(*lua.LTable); ok {
				out = append(out, readProject(pt))
			}
		})
	}
	return out
}

// readProject reads one project card table (also used for the flat fields a
// detail table carries at its top level via a nested `project` table).
func readProject(tbl *lua.LTable) ShopProject {
	if inner, ok := tbl.RawGetString("project").(*lua.LTable); ok {
		tbl = inner
	}
	distribution := ShopDistribution(strings.ToLower(strField(tbl, "distribution")))
	if distribution != ShopDistributionDirect && distribution != ShopDistributionWebsiteOnly {
		distribution = ShopDistributionUnknown
	}
	return ShopProject{
		ID:           strField(tbl, "project_id"),
		Slug:         strField(tbl, "slug"),
		Title:        strField(tbl, "title"),
		Summary:      strField(tbl, "summary"),
		IconURL:      strField(tbl, "icon_url"),
		Author:       strField(tbl, "author"),
		Downloads:    intField(tbl, "downloads"),
		Follows:      intField(tbl, "follows"),
		Categories:   strSlice(tbl, "categories"),
		ProjectType:  strField(tbl, "project_type"),
		Distribution: distribution,
	}
}

// intField reads a numeric field as int64 (0 if absent/non-numeric).
func intField(tbl *lua.LTable, key string) int64 {
	if n, ok := tbl.RawGetString(key).(lua.LNumber); ok {
		return int64(n)
	}
	return 0
}

// boolField reads a boolean field (false if absent).
func boolField(tbl *lua.LTable, key string) bool {
	if b, ok := tbl.RawGetString(key).(lua.LBool); ok {
		return bool(b)
	}
	return false
}

// strSlice reads an array-of-strings field.
func strSlice(tbl *lua.LTable, key string) []string {
	var out []string
	if t, ok := tbl.RawGetString(key).(*lua.LTable); ok {
		t.ForEach(func(_, v lua.LValue) {
			if s, ok := v.(lua.LString); ok && string(s) != "" {
				out = append(out, string(s))
			}
		})
	}
	return out
}
