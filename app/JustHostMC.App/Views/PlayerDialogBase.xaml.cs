using JustHostMC.App.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class PlayerDialogBase : UserControl
{
    public UIElement InnerContent { get; }

    public PlayerDialogBase(PlayerItem player, UIElement innerContent)
    {
        InnerContent = innerContent;
        InitializeComponent();

        HeaderText.Text = player.Name;
        UuidText.Text = string.IsNullOrWhiteSpace(player.Uuid) ? "UUID unknown until the server writes usercache.json." : player.Uuid;
    }

    public void UpdateHeader(string name, string uuid)
    {
        HeaderText.Text = name;
        UuidText.Text = string.IsNullOrWhiteSpace(uuid) ? "UUID unknown until the server writes usercache.json." : uuid;
    }
}
