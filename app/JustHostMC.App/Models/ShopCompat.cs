namespace JustHostMC.App.Models;

/// <summary>Compatibility verdict for a shop listing against the server the
/// shop was opened for. App-local: installed jars get their verdict from the
/// engine (ModMetadata.loader_mismatch/game_version_mismatch); shop listings
/// are evaluated client-side because only the app knows the browsing
/// context.</summary>
public enum ShopCompatVerdict {
    /// <summary>Not evaluated, or neither dimension was checkable.</summary>
    Unknown,
    Ok,
    /// <summary>Built for a different mod loader.</summary>
    LoaderMismatch,
    /// <summary>Built for a different Minecraft version.</summary>
    VersionMismatch,
}

/// <summary>Client-side compatibility check for shop listings. Shop versions
/// carry explicit game-version and loader lists (unlike installed-jar
/// metadata, which may use ranges), so this only needs exact-list membership —
/// no range parser. The loader family table is a deliberate twin of the
/// engine's <c>internal/grpc/modcompat.go</c> table; keep the two in
/// sync.</summary>
public static class ShopCompat {
    private static readonly Dictionary<string, HashSet<string>> LoaderFamilies =
        new(StringComparer.OrdinalIgnoreCase) {
            ["paper"] =
                new(StringComparer.OrdinalIgnoreCase) { "paper", "spigot",
                                                       "bukkit" },
            ["spigot"] =
                new(StringComparer.OrdinalIgnoreCase) { "spigot", "bukkit" },
            ["bukkit"] = new(StringComparer.OrdinalIgnoreCase) { "bukkit" },
            ["fabric"] = new(StringComparer.OrdinalIgnoreCase) { "fabric" },
            ["quilt"] =
                new(StringComparer.OrdinalIgnoreCase) { "quilt", "fabric" },
            ["forge"] = new(
                StringComparer.OrdinalIgnoreCase) { "forge", "forge-legacy" },
            ["neoforge"] = new(StringComparer.OrdinalIgnoreCase) { "neoforge" },
            ["vanilla"]  = new(StringComparer.OrdinalIgnoreCase),
    };

    /// <summary>Verdict for a listing that supports <paramref
    /// name="modLoaders"/> and <paramref name="modGameVersions"/> against a
    /// server on <paramref name="serverLoader"/> / <paramref
    /// name="serverMcVersion"/>. Mirrors the engine's precedence: a confident
    /// loader mismatch first, then a version mismatch, then Ok when at least
    /// one dimension was checkable, else Unknown.</summary>
    public static ShopCompatVerdict Evaluate(
        string serverLoader, string serverMcVersion,
        IReadOnlyList<string> modLoaders,
        IReadOnlyList<string> modGameVersions) {
        var loaderKnown = LoaderFamilies.TryGetValue(
                              (serverLoader ?? "").Trim(), out var family) &&
                          modLoaders.Count > 0;
        if (loaderKnown && !modLoaders.Any(l => family!.Contains(l.Trim())))
            return ShopCompatVerdict.LoaderMismatch;

        var versionKnown =
            !string.IsNullOrEmpty(serverMcVersion) && modGameVersions.Count > 0;
        if (versionKnown &&
            !modGameVersions.Any(v => VersionMatches(serverMcVersion, v)))
            return ShopCompatVerdict.VersionMismatch;

        return loaderKnown || versionKnown ? ShopCompatVerdict.Ok
                                           : ShopCompatVerdict.Unknown;
    }

    // Shop lists are explicit, but tolerate a "1.20.x"/"1.20.*" family pin.
    private static bool VersionMatches(string mc, string listed) {
        listed = listed.Trim();
        if (listed.Length == 0)
            return false;
        if (listed.EndsWith(".x", StringComparison.Ordinal) ||
            listed.EndsWith(".*", StringComparison.Ordinal)) {
            var prefix = listed[..^ 2];
            return mc == prefix ||
                   mc.StartsWith(prefix + ".", StringComparison.Ordinal);
        }
        return string.Equals(mc, listed, StringComparison.OrdinalIgnoreCase);
    }
}
