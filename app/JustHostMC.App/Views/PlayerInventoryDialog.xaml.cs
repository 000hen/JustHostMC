using System.Collections.ObjectModel;
using Grpc.Core;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace JustHostMC.App.Views;

/// <summary>Shows player and Ender Chest contents in Minecraft-style slot grids.</summary>
public sealed partial class PlayerInventoryDialog : FluentContentDialog
{
    private readonly string _serverId;
    private readonly PlayerItem _player;

    public ObservableCollection<PlayerInventoryItemView> MainSlots { get; } = new();
    public ObservableCollection<PlayerInventoryItemView> HotbarSlots { get; } = new();
    public ObservableCollection<PlayerInventoryItemView> EquipmentSlots { get; } = new();
    public ObservableCollection<PlayerInventoryItemView> EnderSlots { get; } = new();

    public PlayerInventoryDialog(string serverId, PlayerItem player)
    {
        _serverId = serverId;
        _player = player;
        InitializeComponent();
        Title = $"Inventory — {player.Name}";
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

            var inventory = await Task.WhenAll(data.Inventory.Select(PlayerInventoryItemView.CreateAsync));
            var ender = await Task.WhenAll(data.EnderChest.Select(PlayerInventoryItemView.CreateAsync));
            var inventoryBySlot = inventory.ToDictionary(item => item.Slot);
            var enderBySlot = ender.ToDictionary(item => item.Slot);

            FillSlots(MainSlots, Enumerable.Range(9, 27), inventoryBySlot, slot => $"Inventory {slot - 8}");
            FillSlots(HotbarSlots, Enumerable.Range(0, 9), inventoryBySlot, slot => $"Hotbar {slot + 1}");
            FillSlots(EquipmentSlots, new[] { 103, 102, 101, 100, -106 }, inventoryBySlot, EquipmentName);
            FillSlots(EnderSlots, Enumerable.Range(0, 27), enderBySlot, slot => $"Ender chest {slot + 1}");
        }
        catch (RpcException ex)
        {
            StatusText.Text = ex.Status.Detail.Length > 0
                ? ex.Status.Detail
                : "Player inventory could not be loaded. The player may need to join once and the server may need to save.";
        }
        finally
        {
            BusyBar.Visibility = Microsoft.UI.Xaml.Visibility.Collapsed;
        }
    }

    private static void FillSlots(
        ObservableCollection<PlayerInventoryItemView> target,
        IEnumerable<int> slots,
        IReadOnlyDictionary<int, PlayerInventoryItemView> items,
        Func<int, string> slotName)
    {
        target.Clear();
        foreach (var slot in slots)
            target.Add(items.TryGetValue(slot, out var item)
                ? item
                : PlayerInventoryItemView.Empty(slot, slotName(slot)));
    }

    private static string EquipmentName(int slot) => slot switch
    {
        103 => "Helmet",
        102 => "Chestplate",
        101 => "Leggings",
        100 => "Boots",
        -106 => "Offhand",
        _ => $"Slot {slot}",
    };

    private void OnInventorySlotClick(object sender, RoutedEventArgs e)
    {
        if (sender is not Button { Tag: PlayerInventoryItemView { HasItem: true } item } button)
            return;

        var header = new Grid { ColumnSpacing = 12 };
        header.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });
        header.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        header.Children.Add(new Image
        {
            Source = item.Icon,
            Width = 52,
            Height = 52,
            Stretch = Stretch.Uniform,
        });
        var heading = new StackPanel { Spacing = 2, VerticalAlignment = VerticalAlignment.Center };
        heading.Children.Add(new TextBlock
        {
            Text = item.StyledName,
            FontSize = 16,
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            TextWrapping = TextWrapping.Wrap,
        });
        heading.Children.Add(new TextBlock
        {
            Text = $"{item.ItemId}  ×{item.Count}",
            FontFamily = new FontFamily("Consolas"),
            FontSize = 12,
            Foreground = (Brush)Application.Current.Resources["TextFillColorSecondaryBrush"],
        });
        Grid.SetColumn(heading, 1);
        header.Children.Add(heading);

        var content = new StackPanel
        {
            Spacing = 12,
            Width = 400,
        };
        content.Children.Add(header);
        content.Children.Add(new TextBlock
        {
            Text = "NBT details",
            Style = (Style)Application.Current.Resources["BodyStrongTextBlockStyle"],
        });
        content.Children.Add(CreateDetailRow("Slot", $"{item.SlotName} ({item.Slot})"));
        content.Children.Add(CreateDetailRow("Item ID", item.ItemId));
        content.Children.Add(CreateDetailRow("Count", item.Count.ToString()));
        foreach (var detail in item.Details)
            content.Children.Add(CreateDetailRow(detail.Label, detail.Value, detail.Label == "Lore"));

        if (item.Details.Count == 0)
        {
            content.Children.Add(new TextBlock
            {
                Text = "No additional item NBT tags.",
                FontStyle = Windows.UI.Text.FontStyle.Italic,
                Foreground = (Brush)Application.Current.Resources["TextFillColorSecondaryBrush"],
            });
        }

        var rawBox = new TextBox
        {
            Text = SnbtFormatter.Format(item.RawSnbt),
            IsReadOnly = true,
            AcceptsReturn = true,
            TextWrapping = TextWrapping.Wrap,
            FontFamily = new FontFamily("Consolas"),
            FontSize = 11,
            MaxHeight = 260,
        };
        content.Children.Add(new Expander
        {
            Header = "Raw item NBT",
            Content = rawBox,
            HorizontalContentAlignment = HorizontalAlignment.Stretch,
        });

        var flyout = new Flyout
        {
            Content = new ScrollViewer
            {
                Content = content,
                MaxHeight = 540,
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
            },
        };
        flyout.ShowAt(button);
    }

    private static UIElement CreateDetailRow(string label, string value, bool italic = false)
    {
        var row = new Grid { ColumnSpacing = 12 };
        row.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(120) });
        row.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        row.Children.Add(new TextBlock
        {
            Text = label,
            FontWeight = Microsoft.UI.Text.FontWeights.SemiBold,
            TextWrapping = TextWrapping.Wrap,
        });
        var valueText = new TextBlock
        {
            Text = value,
            TextWrapping = TextWrapping.Wrap,
            FontStyle = italic ? Windows.UI.Text.FontStyle.Italic : Windows.UI.Text.FontStyle.Normal,
        };
        Grid.SetColumn(valueText, 1);
        row.Children.Add(valueText);
        return row;
    }
}
