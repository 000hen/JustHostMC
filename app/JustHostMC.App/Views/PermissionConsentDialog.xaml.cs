using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Linq;
using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;

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
public sealed partial class PermissionConsentDialog : FluentContentDialog
{
    public ObservableCollection<ConsentRow> Rows { get; } = new();

    public PermissionConsentDialog(
        string scriptName, IEnumerable<Permission> permissions, ILocalizer localizer)
    {
        InitializeComponent();
        Title = localizer.Get("PermissionConsentDialog_Title", ("name", scriptName));

        foreach (var p in permissions)
        {
            // Default-allow each requested permission; the user can deny individually.
            Rows.Add(new ConsentRow(p, localizer, allowed: true));
        }

        // No permissions requested => nothing to weigh; hide the empty list hint logic
        // is handled in XAML via the count converter.
    }

    /// <summary>True when there are no permissions to review.</summary>
    public bool HasNoPermissions => Rows.Count == 0;

    /// <summary>The permission kinds the user chose to allow.</summary>
    public IReadOnlyList<PermissionKind> Granted =>
        Rows.Where(r => r.Allowed).Select(r => r.Kind).ToList();
}
