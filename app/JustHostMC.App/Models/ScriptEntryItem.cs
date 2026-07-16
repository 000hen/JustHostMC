using System.Collections.ObjectModel;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Shared presentation model for an installed provider or automation
/// script.</summary>
public abstract class ScriptEntryItem {
    protected ScriptEntryItem(string id, string name, string author,
                              string version, string description,
                              IEnumerable<Permission> permissions,
                              IEnumerable<PermissionKind> granted) {
        Id             = id;
        Name           = string.IsNullOrEmpty(name) ? id : name;
        Author         = author;
        Version        = version;
        Description    = description;
        Permissions    = permissions.ToList();
        Granted        = granted.ToList();
    }

    public string Id { get; }
    public string Name { get; }
    public string Author { get; }
    public string Version { get; }
    public string Description { get; }
    public IReadOnlyList<Permission> Permissions { get; }
    public IReadOnlyList<PermissionKind> Granted { get; }

    public virtual bool IsBuiltIn      => false;
    public virtual bool CanRemove      => true;
    public virtual bool SupportsToggle => false;
    public virtual bool SupportsLogs   => false;

    /// <summary>Last-known enabled state; used only when <see
    /// cref="SupportsToggle"/> is true.</summary>
    public bool Enabled { get; set; }
    public ObservableCollection<string> LogLines { get; } = new();
}
