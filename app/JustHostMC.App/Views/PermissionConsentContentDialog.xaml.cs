using JustHostMC.App.Controls;
using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class PermissionConsentContentDialog : ContentDialog {
    public PermissionConsentDialog Body { get; }

    public PermissionConsentContentDialog(string scriptName,
                                          IEnumerable<Permission> permissions) {
        Body = new PermissionConsentDialog(permissions);
        InitializeComponent();
        ContentDialogSizing.Apply(this);

        Title = Title?.ToString()?.Replace("{name}", scriptName,
                                           StringComparison.Ordinal);
    }

    public IReadOnlyList<PermissionKind> Granted => Body.Granted;
}
