using System.Collections.Generic;
using System.Linq;
using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>View model for one installed Lua provider in the Scripts page list.</summary>
public sealed class ProviderItem
{
    public ProviderItem(ProviderInfo info, ILocalizer localizer)
    {
        Id = info.Id;
        Name = string.IsNullOrEmpty(info.Name) ? info.Id : info.Name;
        Author = info.Author;
        Version = info.Version;
        Description = info.Description;
        Builtin = info.Builtin;
        Permissions = info.Permissions.ToList();
        Granted = info.Granted.ToList();
        GrantedSummary = string.Join(", ",
            info.Granted.Select(k => PermissionLabels.Label(k, localizer)));
    }

    public string Id { get; }
    public string Name { get; }
    public string Author { get; }
    public string Version { get; }
    public string Description { get; }
    public bool Builtin { get; }

    public IReadOnlyList<Permission> Permissions { get; }
    public IReadOnlyList<PermissionKind> Granted { get; }

    /// <summary>Comma-separated localized labels of the currently-granted permissions.</summary>
    public string GrantedSummary { get; }

    /// <summary>Built-in providers cannot be removed.</summary>
    public bool CanRemove => !Builtin;
}
