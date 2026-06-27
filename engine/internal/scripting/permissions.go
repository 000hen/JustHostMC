// Package scripting embeds a sandboxed Lua engine (gopher-lua) that runs the
// provider scripts which download/install server types and the automation
// scripts that drive running servers. Scripts see only a curated host API
// (jhmc.* / server.*), each capability gated by a permission the user grants.
package scripting

import mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"

// permByName maps the lowercase capability names scripts declare in their
// meta.permissions to the proto PermissionKind enum. The names are the script
// authoring vocabulary; keep them stable.
var permByName = map[string]mcmanagerv1.PermissionKind{
	"network":        mcmanagerv1.PermissionKind_PERMISSION_NETWORK,
	"install":        mcmanagerv1.PermissionKind_PERMISSION_INSTALL,
	"fs_server":      mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER,
	"console_read":   mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ,
	"console_write":  mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_WRITE,
	"server_control": mcmanagerv1.PermissionKind_PERMISSION_SERVER_CONTROL,
	"schedule":       mcmanagerv1.PermissionKind_PERMISSION_SCHEDULE,
}

// permNames is the reverse of permByName for diagnostics/UI fallbacks.
var permNames = func() map[mcmanagerv1.PermissionKind]string {
	m := make(map[mcmanagerv1.PermissionKind]string, len(permByName))
	for name, kind := range permByName {
		m[kind] = name
	}
	return m
}()

// PermName returns the lowercase script-facing name for a permission kind.
func PermName(k mcmanagerv1.PermissionKind) string {
	if name, ok := permNames[k]; ok {
		return name
	}
	return k.String()
}

// GrantSet is the set of permission kinds currently granted to a script.
type GrantSet map[mcmanagerv1.PermissionKind]bool

// Has reports whether kind is granted.
func (g GrantSet) Has(kind mcmanagerv1.PermissionKind) bool { return g != nil && g[kind] }

// grantSetFromList builds a GrantSet from a slice of kinds.
func grantSetFromList(kinds []mcmanagerv1.PermissionKind) GrantSet {
	g := make(GrantSet, len(kinds))
	for _, k := range kinds {
		g[k] = true
	}
	return g
}
