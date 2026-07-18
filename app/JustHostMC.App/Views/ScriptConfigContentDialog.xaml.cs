using JustHostMC.App.Controls;
using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class ScriptConfigContentDialog : ContentDialog {
    public ScriptConfigDialog Body { get; }

    public ScriptConfigContentDialog(string id, string name,
                                     IReadOnlyList<ConfigOption> options,
                                     ScriptConfig current) {
        Body = new ScriptConfigDialog(id, options, current);
        InitializeComponent();
        ContentDialogSizing.Apply(this);

        Title = Title?.ToString()?.Replace("{name}", name,
                                           StringComparison.Ordinal);
    }

    public SetConfigRequest BuildRequest() => Body.BuildRequest();
}
