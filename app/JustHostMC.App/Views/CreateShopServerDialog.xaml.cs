using JustHostMC.App.Controls;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class CreateShopServerDialog : ContentDialog {
    public CreateShopServerDialog(string projectName) {
        ProjectName = projectName;
        InitializeComponent();
        ContentDialogSizing.Apply(this);
        NameBox.Text = projectName;
    }

    public string ProjectName { get; }

    public string ServerName => NameBox.Text.Trim().Length >
                                0? NameBox.Text.Trim() : ProjectName;

    public int MemoryMb =>
        double.IsNaN(MemoryBox.Value) ? 4096 : (int)MemoryBox.Value;
}
