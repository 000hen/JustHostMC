using System.Collections.Generic;
using System.Text.RegularExpressions;
using McManager.Grpc;

namespace JustHostMC.App.Services;

/// <summary>
/// Best-effort client-side parser for the permissions a Lua script declares in
/// its <c>meta.permissions</c> table, so the consent dialog can show them
/// BEFORE import. The engine re-parses and enforces the real declaration; this
/// is UI-only.
/// </summary>
public static partial class LuaPermissions {
    // Maps the lowercase script-facing names (see engine permissions.go) to
    // enum kinds.
    private static readonly Dictionary<string, PermissionKind> ByName = new() {
        ["network"]        = PermissionKind.PermissionNetwork,
        ["install"]        = PermissionKind.PermissionInstall,
        ["fs_server"]      = PermissionKind.PermissionFsServer,
        ["console_read"]   = PermissionKind.PermissionConsoleRead,
        ["console_write"]  = PermissionKind.PermissionConsoleWrite,
        ["server_control"] = PermissionKind.PermissionServerControl,
        ["schedule"]       = PermissionKind.PermissionSchedule,
        ["server_query"]   = PermissionKind.PermissionServerQuery,
        ["player_manage"]  = PermissionKind.PermissionPlayerManage,
    };

    // Matches one `{ kind = "name", reason = "text" }` entry (reason optional,
    // order-agnostic).
    [GeneratedRegex(
        @"\{\s*(?=[^}]*kind\s*=\s*[""']([a-z_]+)[""'])(?:[^}]*reason\s*=\s*[""']((?:\\.|[^""'\\])*)[""'])?[^}]*\}",
        RegexOptions.IgnoreCase | RegexOptions.Singleline)]
    private static partial Regex EntryRegex();

    /// <summary>Parses declared permissions from a Lua script's source text.
    /// Unknown permission names are ignored.</summary>
    public static IReadOnlyList<Permission> Parse(string luaSource) {
        var result = new List<Permission>();
        if (string.IsNullOrEmpty(luaSource))
            return result;

        var seen = new HashSet<PermissionKind>();
        foreach (Match m in EntryRegex().Matches(luaSource)) {
            var name = m.Groups[1].Value.ToLowerInvariant();
            if (!ByName.TryGetValue(name, out var kind) || !seen.Add(kind))
                continue;
            var reason = m.Groups[2].Success ? Unescape(m.Groups[2].Value) : "";
            result.Add(new Permission { Kind = kind, Reason = reason });
        }
        return result;
    }

    private static string Unescape(string s) =>
        s.Replace("\\\"", "\"").Replace("\\'", "'").Replace("\\\\", "\\");
}
