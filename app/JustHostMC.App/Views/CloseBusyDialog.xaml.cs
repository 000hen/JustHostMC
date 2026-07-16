using JustHostMC.App.Controls;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class CloseBusyDialog : ContentDialog {
    public CloseBusyDialog() {
        InitializeComponent();
        ContentDialogSizing.Apply(this);
    }
}
