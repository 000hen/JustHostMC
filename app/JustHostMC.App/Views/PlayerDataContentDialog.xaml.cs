using JustHostMC.App.Models;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class PlayerDataContentDialog : ContentDialog {
    public PlayerDialogBase Body { get; }

    public PlayerDataContentDialog(string serverId, PlayerItem player) {
        var view = new PlayerDataDialog(serverId, player);
        Body = new PlayerDialogBase(player, view);
        InitializeComponent();

        Title = string.Format(Title?.ToString() ?? "{0}", player.Name);
        view.OnHeaderUpdated = Body.UpdateHeader;
    }
}
