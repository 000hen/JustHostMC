using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>One requested permission row with an allow/deny toggle, bound in
/// the consent dialog.</summary>
public sealed partial class ConsentRow : ObservableObject {
    public ConsentRow(Permission permission, bool allowed) {
        Kind    = permission.Kind;
        Reason  = permission.Reason;
        Allowed = allowed;
    }

    public PermissionKind Kind { get; }
    public string Reason { get; }

    [ObservableProperty]
    public partial bool Allowed {
        get; set;
    }
}

/// <summary>Consent dialog: shows each requested permission with its reason and
/// an allow/deny toggle. On primary, <see cref="Granted"/> holds the allowed
/// kinds.</summary>
public sealed partial class PermissionConsentDialog : UserControl {
    public ObservableCollection<ConsentRow> Rows { get; } = new();

    public PermissionConsentDialog(IEnumerable<Permission> permissions) {
        // Populate before InitializeComponent so the OneTime x:Bind to
        // Rows.Count (used to toggle the "no permissions" hint) sees the final
        // count.
        foreach (var p in permissions) {
            // Default-allow each requested permission; the user can deny
            // individually.
            Rows.Add(new ConsentRow(p, allowed: true));
        }

        InitializeComponent();
    }

    /// <summary>The permission kinds the user chose to allow.</summary>
    public IReadOnlyList<PermissionKind> Granted =>
        Rows.Where(r => r.Allowed).Select(r => r.Kind).ToList();
}
