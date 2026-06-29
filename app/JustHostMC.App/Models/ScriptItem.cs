using System.Collections.Generic;
using System.Linq;
using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>View model for one installed automation script in the Scripts page list.</summary>
public sealed class ScriptItem
{
    public ScriptItem(ScriptInfo info, ILocalizer localizer)
    {
        Id = info.Id;
        Name = string.IsNullOrEmpty(info.Name) ? info.Id : info.Name;
        Author = info.Author;
        Version = info.Version;
        Description = info.Description;
        Enabled = info.Enabled;
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

    /// <summary>Last-known enabled state from the engine; the toggle drives SetEnabled.
    /// Updated after a successful toggle so the page handler can ignore no-op events
    /// (the ToggleSwitch raises Toggled when its IsOn binding is first applied).</summary>
    public bool Enabled { get; set; }

    public IReadOnlyList<Permission> Permissions { get; }
    public IReadOnlyList<PermissionKind> Granted { get; }

    /// <summary>Comma-separated localized labels of the currently-granted permissions.</summary>
    public string GrantedSummary { get; }
}
