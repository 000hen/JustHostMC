package scripting

import (
	"fmt"
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

	return m, nil
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

// strField reads a string field from a Lua table, returning "" if absent.
func strField(tbl *lua.LTable, key string) string {
	if s, ok := tbl.RawGetString(key).(lua.LString); ok {
		return strings.TrimSpace(string(s))
	}
	return ""
}
