using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Display row for a player inventory or ender chest item.</summary>
public sealed class PlayerInventoryItemView
{
    public PlayerInventoryItemView(PlayerInventoryItem item)
    {
        Slot = item.Slot;
        SlotName = item.SlotName;
        ItemId = item.ItemId;
        Count = item.Count;
        RawSnbt = item.RawSnbt;
    }

    public int Slot { get; }
    public string SlotName { get; }
    public string ItemId { get; }
    public int Count { get; }
    public string RawSnbt { get; }
}
