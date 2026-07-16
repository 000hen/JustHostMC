using JustHostMC.App.Controls;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class ServerFolderNotFoundDialog : ContentDialog {
    public ServerFolderNotFoundDialog() {
        InitializeComponent();
        ContentDialogSizing.Apply(this);
    }
}
