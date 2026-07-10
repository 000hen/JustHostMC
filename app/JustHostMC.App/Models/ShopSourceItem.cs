using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>View model for one installed mod shop source (a remote catalog such
/// as Modrinth or CurseForge) in the Scripts page list.</summary>
public sealed class ShopSourceItem : ScriptEntryItem {
    public ShopSourceItem(ShopInfo info, ILocalizer localizer)
        : base(info.Id, info.Name, info.Author, info.Version,
               ComposeDescription(info, localizer), info.Permissions,
               info.Granted, info.ConfigOptions, localizer) {
        Builtin  = info.Builtin;
        NeedsKey = info.NeedsKey;
        Ready    = info.Ready;
    }

    public bool Builtin { get; }

    /// <summary>The shop declares it needs a per-shop API key.</summary>
    public bool NeedsKey { get; }

    /// <summary>False only when the shop needs a key and none is configured
    /// yet.</summary>
    public bool Ready { get; }

    public override bool IsBuiltIn => Builtin;

    /// <summary>Built-in shop sources cannot be removed.</summary>
    public override bool CanRemove => !Builtin;

    /// <summary>Appends a "needs API key" note to the description when the shop
    /// requires a key but none is configured, so the shared entry card surfaces
    /// it.</summary>
    private static string ComposeDescription(ShopInfo info,
                                             ILocalizer localizer) {
        if (!info.NeedsKey || info.Ready)
            return info.Description;
        var note = localizer.Get("Scripts_ShopNeedsKey");
        return info.Description.Length == 0 ? note
                                            : $"{info.Description}\n{note}";
    }
}
