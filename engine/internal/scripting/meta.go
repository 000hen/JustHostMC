package scripting

import (
	"fmt"
	"strconv"
	"strings"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	lua "github.com/yuin/gopher-lua"
)

// Permission is a capability a script declares it needs, with a human reason
// shown in the consent dialog.
type Permission struct {
	Kind   mcmanagerv1.PermissionKind
	Reason string
}

// ConfigOption is one author-declared typed config field (a meta.config entry).
// The same shape backs every scriptable subsystem (providers, automation
// scripts, parsers and shops).
type ConfigOption struct {
	Key         string
	Type        mcmanagerv1.ConfigOptionType
	Name        string
	Description string
	Default     string // string-encoded; validated to parse for number/boolean
	Required    bool
}

// configTypeByName maps a lowercase meta.config `type` string to its enum.
var configTypeByName = map[string]mcmanagerv1.ConfigOptionType{
	"string":  mcmanagerv1.ConfigOptionType_CONFIG_OPTION_STRING,
	"number":  mcmanagerv1.ConfigOptionType_CONFIG_OPTION_NUMBER,
	"boolean": mcmanagerv1.ConfigOptionType_CONFIG_OPTION_BOOLEAN,
	"secret":  mcmanagerv1.ConfigOptionType_CONFIG_OPTION_SECRET,
}

// Meta is the author-declared header every provider/automation script carries.
type Meta struct {
	ID          string
	Name        string
	Website     string
	Description string
	Version     string
	Author      string
	// ModLayout is "plugins", "mods", or "none" (providers only). Drives the
	// per-server mods/plugins UI without server-type-specific code.
	ModLayout   string
	Permissions []Permission
	// Formats lists the descriptor files a parser script reads (parsers only),
	// e.g. "fabric.mod.json"; shown in the parser management UI.
	Formats []string
	// NeedsKey marks a shop script whose source requires an API key
	// (shops only); the shop is not Ready until one is configured.
	NeedsKey bool
	// Kinds lists the item kinds a shop serves ("mod"/"plugin"/"modpack");
	// shops only. Empty means the default {"mod","plugin"} (applied by the
	// shop service, not here, so parseMeta stays subsystem-agnostic).
	Kinds []string
	// Hidden marks a provider not offered in the create-server UI — its install
	// is driven elsewhere (e.g. a modpack shop); providers only.
	Hidden bool
	// Config lists the author-declared typed config options (all subsystems).
	Config []ConfigOption
}

// DeclaredKinds returns just the permission kinds the script declares.
func (m Meta) DeclaredKinds() []mcmanagerv1.PermissionKind {
	out := make([]mcmanagerv1.PermissionKind, 0, len(m.Permissions))
	for _, p := range m.Permissions {
		out = append(out, p.Kind)
	}
	return out
}

// parseMeta reads the global `meta` table left by a loaded script.
func parseMeta(L *lua.LState) (Meta, error) {
	v := L.GetGlobal("meta")
	tbl, ok := v.(*lua.LTable)
	if !ok {
		return Meta{}, fmt.Errorf("script has no `meta` table")
	}

	m := Meta{
		ID:          strField(tbl, "id"),
		Name:        strField(tbl, "name"),
		Website:     strField(tbl, "website"),
		Description: strField(tbl, "description"),
		Version:     strField(tbl, "version"),
		Author:      strField(tbl, "author"),
		ModLayout:   strings.ToLower(strField(tbl, "mod_layout")),
	}
	if m.ID == "" {
		return Meta{}, fmt.Errorf("script meta.id is required")
	}
	if !validProviderID(m.ID) {
		return Meta{}, fmt.Errorf("script meta.id %q must contain only letters, digits, '-' or '_'", m.ID)
	}
	if m.ModLayout == "" {
		m.ModLayout = "none"
	}
	if b, ok := tbl.RawGetString("needs_key").(lua.LBool); ok {
		m.NeedsKey = bool(b)
	}

	if b, ok := tbl.RawGetString("hidden").(lua.LBool); ok {
		m.Hidden = bool(b)
	}

	if kinds, ok := tbl.RawGetString("kinds").(*lua.LTable); ok {
		var kerr error
		kinds.ForEach(func(_, kv lua.LValue) {
			if kerr != nil {
				return
			}
			s, ok := kv.(lua.LString)
			if !ok {
				kerr = fmt.Errorf("meta.kinds entries must be strings")
				return
			}
			kind := strings.ToLower(strings.TrimSpace(string(s)))
			if !validShopKind(kind) {
				kerr = fmt.Errorf("meta.kinds has unknown kind %q", kind)
				return
			}
			m.Kinds = append(m.Kinds, kind)
		})
		if kerr != nil {
			return Meta{}, kerr
		}
	}

	if formats, ok := tbl.RawGetString("formats").(*lua.LTable); ok {
		formats.ForEach(func(_, fv lua.LValue) {
			if s, ok := fv.(lua.LString); ok && strings.TrimSpace(string(s)) != "" {
				m.Formats = append(m.Formats, strings.TrimSpace(string(s)))
			}
		})
	}

	if perms, ok := tbl.RawGetString("permissions").(*lua.LTable); ok {
		var perr error
		perms.ForEach(func(_, pv lua.LValue) {
			if perr != nil {
				return
			}
			ptbl, ok := pv.(*lua.LTable)
			if !ok {
				perr = fmt.Errorf("meta.permissions entries must be tables")
				return
			}
			name := strings.ToLower(strField(ptbl, "kind"))
			kind, ok := permByName[name]
			if !ok {
				perr = fmt.Errorf("unknown permission kind %q", name)
				return
			}
			m.Permissions = append(m.Permissions, Permission{Kind: kind, Reason: strField(ptbl, "reason")})
		})
		if perr != nil {
			return Meta{}, perr
		}
	}

	if cfg, ok := tbl.RawGetString("config").(*lua.LTable); ok {
		var cerr error
		seen := map[string]bool{}
		cfg.ForEach(func(_, cv lua.LValue) {
			if cerr != nil {
				return
			}
			ctbl, ok := cv.(*lua.LTable)
			if !ok {
				cerr = fmt.Errorf("meta.config entries must be tables")
				return
			}
			key := strField(ctbl, "key")
			if key == "" {
				cerr = fmt.Errorf("meta.config entry is missing a key")
				return
			}
			if !validProviderID(key) {
				cerr = fmt.Errorf("meta.config key %q must contain only letters, digits, '-' or '_'", key)
				return
			}
			if seen[key] {
				cerr = fmt.Errorf("meta.config key %q is declared more than once", key)
				return
			}
			seen[key] = true
			typeName := strings.ToLower(strField(ctbl, "type"))
			if typeName == "" {
				typeName = "string"
			}
			ct, ok := configTypeByName[typeName]
			if !ok {
				cerr = fmt.Errorf("meta.config key %q has unknown type %q", key, typeName)
				return
			}
			def := strField(ctbl, "default")
			if err := validateConfigDefault(ct, def); err != nil {
				cerr = fmt.Errorf("meta.config key %q: %w", key, err)
				return
			}
			var required bool
			if b, ok := ctbl.RawGetString("required").(lua.LBool); ok {
				required = bool(b)
			}
			m.Config = append(m.Config, ConfigOption{
				Key:         key,
				Type:        ct,
				Name:        strField(ctbl, "name"),
				Description: strField(ctbl, "description"),
				Default:     def,
				Required:    required,
			})
		})
		if cerr != nil {
			return Meta{}, cerr
		}
	}

	return m, nil
}

// validateConfigDefault checks that a declared default parses for the typed
// options (number/boolean); string/secret accept any default.
func validateConfigDefault(t mcmanagerv1.ConfigOptionType, def string) error {
	if def == "" {
		return nil
	}
	switch t {
	case mcmanagerv1.ConfigOptionType_CONFIG_OPTION_NUMBER:
		if _, err := strconv.ParseFloat(def, 64); err != nil {
			return fmt.Errorf("default %q is not a number", def)
		}
	case mcmanagerv1.ConfigOptionType_CONFIG_OPTION_BOOLEAN:
		if _, err := strconv.ParseBool(def); err != nil {
			return fmt.Errorf("default %q is not a boolean", def)
		}
	}
	return nil
}

// validProviderID reports whether id is a safe path component (letters, digits,
// '-', '_'), so it can be used directly as a directory name by Import/Remove.
func validProviderID(id string) bool {
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return id != ""
}

// validShopKind reports whether k is a recognized shop item kind.
func validShopKind(k string) bool {
	switch k {
	case "mod", "plugin", "modpack":
		return true
	}
	return false
}

// strField reads a string field from a Lua table, returning "" if absent.
func strField(tbl *lua.LTable, key string) string {
	if s, ok := tbl.RawGetString(key).(lua.LString); ok {
		return strings.TrimSpace(string(s))
	}
	return ""
}
