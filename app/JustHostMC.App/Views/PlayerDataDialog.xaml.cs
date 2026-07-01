using Grpc.Core;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Shows one player's raw NBT in a dedicated, readable dialog.</summary>
public sealed partial class PlayerDataDialog : FluentContentDialog
{
    private readonly string _serverId;
    private readonly PlayerItem _player;

    public PlayerDataDialog(string serverId, PlayerItem player)
    {
        _serverId = serverId;
        _player = player;
        InitializeComponent();
        Title = $"Raw player data — {player.Name}";
        HeaderText.Text = player.Name;
        UuidText.Text = string.IsNullOrWhiteSpace(player.Uuid)
            ? "UUID unknown until the server writes usercache.json."
            : player.Uuid;
        Opened += async (_, _) => await LoadAsync();
    }

    private async Task LoadAsync()
    {
        BusyBar.Visibility = Microsoft.UI.Xaml.Visibility.Visible;
        StatusText.Text = "";
        try
        {
            var daemon = await App.Current.DaemonReady;
            var data = await daemon.Players.GetDataAsync(new PlayerLookup
            {
                ServerId = _serverId,
                Name = _player.Name,
                Uuid = _player.Uuid,
            });

            HeaderText.Text = data.Name.Length > 0 ? data.Name : _player.Name;
            UuidText.Text = data.Uuid;
            RawBox.Text = SnbtFormatter.Format(data.RawSnbt);
        }
        catch (RpcException ex)
        {
            StatusText.Text = ex.Status.Detail.Length > 0
                ? ex.Status.Detail
                : "Player data could not be loaded. The player may need to join once and the server may need to save.";
        }
        finally
        {
            BusyBar.Visibility = Microsoft.UI.Xaml.Visibility.Collapsed;
        }
    }

}
