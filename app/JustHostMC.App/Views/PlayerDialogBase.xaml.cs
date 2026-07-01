using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class PlayerDialogBase : FluentContentDialog
{
    public UIElement InnerContent { get; }

    public PlayerDialogBase(string actionName, PlayerItem player, UIElement innerContent)
    {
        InnerContent = innerContent;
        InitializeComponent();
        
        string format;
        try
        {
            format = new Microsoft.Windows.ApplicationModel.Resources.ResourceLoader().GetString("PlayerDialogBase_TitleFormat");
        }
        catch
        {
            format = "{0} for {1}";
        }
        if (string.IsNullOrEmpty(format)) format = "{0} for {1}";

        Title = string.Format(format, actionName, player.Name);
        
        HeaderText.Text = player.Name;
        UuidText.Text = string.IsNullOrWhiteSpace(player.Uuid) ? "UUID unknown until the server writes usercache.json." : player.Uuid;
    }

    public void UpdateHeader(string name, string uuid)
    {
        HeaderText.Text = name;
        UuidText.Text = string.IsNullOrWhiteSpace(uuid) ? "UUID unknown until the server writes usercache.json." : uuid;
    }
}

