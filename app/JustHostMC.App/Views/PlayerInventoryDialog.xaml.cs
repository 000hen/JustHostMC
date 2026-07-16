using System.Collections.ObjectModel;
using Grpc.Core;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Shows player and Ender Chest contents in Minecraft-style slot
/// grids.</summary>
public sealed partial class PlayerInventoryDialog : UserControl {
    private readonly string _serverId;
    private readonly PlayerItem _player;
    private readonly ILocalizer _localizer = new LocalizationService();
    public Action<string, string>? OnHeaderUpdated { get; set; }

    public ObservableCollection<PlayerInventoryItemView> MainSlots {
        get;
    } = new();
    public ObservableCollection<PlayerInventoryItemView> HotbarSlots {
        get;
    } = new();
    public ObservableCollection<PlayerInventoryItemView> EquipmentSlots {
        get;
    } = new();
    public ObservableCollection<PlayerInventoryItemView> EnderSlots {
        get;
    } = new();
    public IReadOnlyList<int> LoadingEquipmentSlots {
        get;
    } = Enumerable.Range(0, 5).ToArray();
    public IReadOnlyList<int> LoadingInventorySlots {
        get;
    } = Enumerable.Range(0, 36).ToArray();
    public IReadOnlyList<int> LoadingEnderSlots {
        get;
    } = Enumerable.Range(0, 27).ToArray();

    public PlayerInventoryDialog(string serverId, PlayerItem player) {
        _serverId = serverId;
        _player   = player;
        InitializeComponent();
        Loaded += async (_, _) => await LoadAsync();
    }

    private async Task LoadAsync() {
        BusyBar.Visibility          = Microsoft.UI.Xaml.Visibility.Visible;
        LoadingSkeleton.Visibility  = Visibility.Visible;
        InventoryContent.Visibility = Visibility.Collapsed;
        LoadFailedText.Visibility   = Visibility.Collapsed;
        try {
            var daemon = await App.Current.DaemonReady;
            var data   = await daemon.Players.GetDataAsync(new PlayerLookup {
                ServerId = _serverId,
                Name     = _player.Name,
                Uuid     = _player.Uuid,
            });

            OnHeaderUpdated?.Invoke(
                data.Name.Length > 0 ? data.Name : _player.Name, data.Uuid);

            var inventory = await Task.WhenAll(
                data.Inventory.Select(PlayerInventoryItemView.CreateAsync));
            var ender = await Task.WhenAll(
                data.EnderChest.Select(PlayerInventoryItemView.CreateAsync));
            var inventoryBySlot = inventory.ToDictionary(item => item.Slot);
            var enderBySlot     = ender.ToDictionary(item => item.Slot);

            var inventorySlotFormat = _localizer.Get("player.inventory.slot");
            var hotbarSlotFormat = _localizer.Get("player.inventory.hotbar");
            var enderSlotFormat = _localizer.Get("player.inventory.ender_slot");
            FillSlots(MainSlots, Enumerable.Range(9, 27), inventoryBySlot,
                      slot => string.Format(inventorySlotFormat, slot - 8));
            FillSlots(HotbarSlots, Enumerable.Range(0, 9), inventoryBySlot,
                      slot => string.Format(hotbarSlotFormat, slot + 1));
            FillSlots(EquipmentSlots, new[] { 103, 102, 101, 100, -106 },
                      inventoryBySlot, EquipmentName);
            FillSlots(EnderSlots, Enumerable.Range(0, 27), enderBySlot,
                      slot => string.Format(enderSlotFormat, slot + 1));
        } catch (RpcException) {
            LoadFailedText.Visibility = Visibility.Visible;
        } finally {
            BusyBar.Visibility         = Microsoft.UI.Xaml.Visibility.Collapsed;
            LoadingSkeleton.Visibility = Visibility.Collapsed;
            InventoryContent.Visibility = Visibility.Visible;
        }
    }

    private static void FillSlots(
        ObservableCollection<PlayerInventoryItemView> target,
        IEnumerable<int> slots,
        IReadOnlyDictionary<int, PlayerInventoryItemView> items,
        Func<int, string> slotName) {
        target.Clear();
        foreach (var slot in slots)
            target.Add(
                items.TryGetValue(slot, out var item)
                    ? item
                    : PlayerInventoryItemView.Empty(slot, slotName(slot)));
    }

    private string EquipmentName(int slot) {
        var key = slot switch {
            103  => "player.inventory.helmet",
            102  => "player.inventory.chestplate",
            101  => "player.inventory.leggings",
            100  => "player.inventory.boots",
            -106 => "player.inventory.offhand",
            _    => "",
        };
        return key.Length > 0
                   ? _localizer.Get(key)
                   : string.Format(
                       _localizer.Get("player.inventory.generic_slot"), slot);
    }

    private void OnInventorySlotClick(object sender, RoutedEventArgs e) {
        if (sender is not Button {
                Tag : PlayerInventoryItemView { HasItem : true } item
            } button)
            return;

        var flyout = new Flyout {
            Content =
                new ScrollViewer {
                                  Content                     = new InventoryItemDetails(item),
                                  MaxHeight                   = 540,
                                  HorizontalContentAlignment  = HorizontalAlignment.Stretch,
                                  VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                                  },
        };
        flyout.ShowAt(button);
    }
}
