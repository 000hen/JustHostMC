using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Linq;
using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>One requested permission row with an allow/deny toggle, bound in the
/// consent dialog. Uses manual SetProperty (not [ObservableProperty]).</summary>
public sealed class ConsentRow : ObservableObject
{
    public ConsentRow(Permission permission, ILocalizer localizer, bool allowed)
    {
        Kind = permission.Kind;
        Label = PermissionLabels.Label(permission.Kind, localizer);
        Reason = permission.Reason;
        _allowed = allowed;
    }

    public PermissionKind Kind { get; }
    public string Label { get; }
    public string Reason { get; }

    private bool _allowed;
    public bool Allowed
    {
        get => _allowed;
        set => SetProperty(ref _allowed, value);
    }
}

/// <summary>Consent dialog: shows each requested permission with its reason and an
/// allow/deny toggle. On primary, <see cref="Granted"/> holds the allowed kinds.</summary>
public sealed partial class PermissionConsentDialog : UserControl
{
    public ObservableCollection<ConsentRow> Rows { get; } = new();

    public PermissionConsentDialog(IEnumerable<Permission> permissions, ILocalizer localizer)
    {
        // Populate before InitializeComponent so the OneTime x:Bind to Rows.Count
        // (used to toggle the "no permissions" hint) sees the final count.
        foreach (var p in permissions)
        {
            // Default-allow each requested permission; the user can deny individually.
            Rows.Add(new ConsentRow(p, localizer, allowed: true));
        }

        InitializeComponent();
    }

    /// <summary>The permission kinds the user chose to allow.</summary>
    public IReadOnlyList<PermissionKind> Granted =>
        Rows.Where(r => r.Allowed).Select(r => r.Kind).ToList();
}
