using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class RenameServerDialog : ContentDialog {
    public RenameServerDialog(string serverName) {
        InitializeComponent();
        NameBox.Text            = serverName;
        NameBox.SelectionStart  = 0;
        NameBox.SelectionLength = serverName.Length;
    }

    public string ServerName => NameBox.Text;
}
