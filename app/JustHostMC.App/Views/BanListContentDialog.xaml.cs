using JustHostMC.App.Controls;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class BanListContentDialog : ContentDialog {
    public BanListDialog Body { get; }

    public BanListContentDialog(string serverId, bool canModify) {
        Body = new BanListDialog(serverId, canModify);
        InitializeComponent();
        ContentDialogSizing.Apply(this, useWideLayout: true);
        Opened += async (_, _) => await Body.LoadAsync();
    }
}
