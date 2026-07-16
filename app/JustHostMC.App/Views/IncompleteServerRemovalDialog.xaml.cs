using JustHostMC.App.Controls;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class IncompleteServerRemovalDialog : ContentDialog {
    public IncompleteServerRemovalDialog() {
        InitializeComponent();
        ContentDialogSizing.Apply(this);
    }
}
