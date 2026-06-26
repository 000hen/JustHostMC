using System.Collections.ObjectModel;
using Grpc.Core;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Shows structured inventory plus raw SNBT for one playerdata file.</summary>
public sealed partial class PlayerDataDialog : FluentContentDialog
{
    private readonly string _serverId;
    private readonly PlayerItem _player;

    public ObservableCollection<PlayerInventoryItemView> Inventory { get; } = new();
    public ObservableCollection<PlayerInventoryItemView> EnderChest { get; } = new();

    public string InventoryHeader => $"Inventory ({Inventory.Count})";
    public string EnderHeader => $"Ender chest ({EnderChest.Count})";

    public PlayerDataDialog(string serverId, PlayerItem player)
    {
        _serverId = serverId;
        _player = player;
        InitializeComponent();
        Title = player.Name;
        HeaderText.Text = player.Name;
        UuidText.Text = string.IsNullOrWhiteSpace(player.Uuid) ? "UUID unknown until the server writes usercache.json." : player.Uuid;
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

            Inventory.Clear();
            foreach (var item in data.Inventory)
                Inventory.Add(new PlayerInventoryItemView(item));
            EnderChest.Clear();
            foreach (var item in data.EnderChest)
                EnderChest.Add(new PlayerInventoryItemView(item));

            HeaderText.Text = data.Name.Length > 0 ? data.Name : _player.Name;
            UuidText.Text = data.Uuid;
            RawBox.Text = data.RawSnbt;
            Bindings.Update();
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
