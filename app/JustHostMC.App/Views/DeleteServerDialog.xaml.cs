using JustHostMC.App.Controls;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class DeleteServerDialog : ContentDialog {
    public DeleteServerDialog() {
        InitializeComponent();
        ContentDialogSizing.Apply(this);
    }
}
