using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>View model for one installed Lua provider in the Scripts page
/// list.</summary>
public sealed class ProviderItem : ScriptEntryItem {
    public ProviderItem(ProviderInfo info, ILocalizer localizer)
        : base(info.Id, info.Name, info.Author, info.Version, info.Description,
               info.Permissions, info.Granted) {
        Builtin = info.Builtin;
    }

    public bool Builtin { get; }
    public override bool IsBuiltIn => Builtin;

    /// <summary>Built-in providers cannot be removed.</summary>
    public override bool CanRemove => !Builtin;
}
