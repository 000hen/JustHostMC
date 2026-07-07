using System.Collections.ObjectModel;
using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Shared presentation model for an installed provider or automation
/// script.</summary>
public abstract class ScriptEntryItem {
    protected ScriptEntryItem(string id, string name, string author,
                              string version, string description,
                              IEnumerable<Permission> permissions,
                              IEnumerable<PermissionKind> granted,
                              ILocalizer localizer) {
        Id             = id;
        Name           = string.IsNullOrEmpty(name) ? id : name;
        Author         = author;
        Version        = version;
        Description    = description;
        Permissions    = permissions.ToList();
        Granted        = granted.ToList();
        GrantedSummary = string.Join(
            ", ",
            Granted.Select(kind => PermissionLabels.Label(kind, localizer)));
    }

    public string Id { get; }
    public string Name { get; }
    public string Author { get; }
    public string Version { get; }
    public string Description { get; }
    public IReadOnlyList<Permission> Permissions { get; }
    public IReadOnlyList<PermissionKind> Granted { get; }
    public string GrantedSummary { get; }

    public virtual bool IsBuiltIn      => false;
    public virtual bool CanRemove      => true;
    public virtual bool SupportsToggle => false;
    public virtual bool SupportsLogs   => false;

    /// <summary>Last-known enabled state; used only when <see
    /// cref="SupportsToggle"/> is true.</summary>
    public bool Enabled { get; set; }
    public ObservableCollection<string> LogLines { get; } = new();
}
