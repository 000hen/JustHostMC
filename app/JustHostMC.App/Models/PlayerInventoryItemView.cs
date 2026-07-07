using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml.Media;

namespace JustHostMC.App.Models;

/// <summary>A Minecraft-style inventory slot, populated or empty.</summary>
public sealed class PlayerInventoryItemView {
    private PlayerInventoryItemView(int slot, string slotName) {
        Slot        = slot;
        SlotName    = slotName;
        ItemId      = "";
        DisplayName = "";
        RawSnbt     = "";
        Details     = Array.Empty<PlayerItemDetail>();
        Nbt         = MinecraftItemNbtParser.Parse("");
    }

    private PlayerInventoryItemView(PlayerInventoryItem item) {
        Slot        = item.Slot;
        SlotName    = item.SlotName;
        ItemId      = item.ItemId;
        Count       = item.Count;
        RawSnbt     = item.RawSnbt;
        Details     = item.Details.ToList();
        Nbt         = MinecraftItemNbtParser.Parse(item.RawNbt.ToByteArray(),
                                                   item.RawSnbt);
        var name    = item.ItemId.Contains(':')
                          ? item.ItemId[(item.ItemId.IndexOf(':') + 1)..]
                          : item.ItemId;
        DisplayName = string.Join(' ', name.Split('_').Select(WordCase));
    }

    public int Slot { get; }
    public string SlotName { get; }
    public string ItemId { get; }
    public string DisplayName { get; }
    public int Count { get; }
    public string RawSnbt { get; }
    public IReadOnlyList<PlayerItemDetail> Details { get; }
    internal MinecraftItemNbtPresentation Nbt { get; }
    public string StyledName => Nbt.DisplayName ?? DisplayName;
    public ImageSource? Icon { get; private set; }
    public bool HasItem          => ItemId.Length > 0;
    public bool HasIcon          => Icon is not null;
    public bool ShowFallbackIcon => HasItem && !HasIcon;
    public bool ShowCount        => Count > 1;
    public string CountText      => ShowCount ? Count.ToString() : "";
    public string Tooltip =>
        HasItem ? $"{StyledName}\n{ItemId}\n{SlotName}" : SlotName;

    public static PlayerInventoryItemView Empty(int slot, string slotName) =>
        new(slot, slotName);

    public static async Task<PlayerInventoryItemView> CreateAsync(
        PlayerInventoryItem item) {
        var view = new PlayerInventoryItemView(item);
        try {
            view.Icon = await MinecraftItemIconRenderer.RenderAsync(
                item.ItemId, item.RenderAssets);
        } catch {
            // Keep the slot usable with its local fallback glyph and tooltip.
        }
        return view;
    }

    private static string WordCase(string value) =>
        value.Length == 0 ? value
                          : char.ToUpperInvariant(value[0]) + value[1..];
}
