using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Bindable ban-list row from banned-players.json or
/// banned-ips.json.</summary>
public sealed class BanEntryItem {
    public BanEntryItem(BanEntry entry, ILocalizer localizer) {
        Type    = entry.Type;
        Target  = entry.Target;
        Name    = entry.Name;
        Uuid    = entry.Uuid;
        Created = entry.Created;
        Source  = entry.Source;
        Expires = entry.Expires;
        Reason  = entry.Reason;
        TypeText =
            localizer.Get(Type == BanListType.IpBans ? "BanList_TypeIp"
                                                     : "BanList_TypePlayer");
        if (string.IsNullOrWhiteSpace(TypeText))
            TypeText = Type == BanListType.IpBans ? "IP" : "Player";
        ReasonText = string.IsNullOrWhiteSpace(Reason)
                         ? localizer.Get("BanList_NoReason")
                         : Reason;
        if (string.IsNullOrWhiteSpace(ReasonText))
            ReasonText = string.IsNullOrWhiteSpace(Reason)
                             ? "No reason provided"
                             : Reason;
        IdentityText = !string.IsNullOrWhiteSpace(Uuid) ? Uuid : Source;
        CreatedText  = string.IsNullOrWhiteSpace(Created) ? Expires : Created;
    }

    public BanListType Type { get; }
    public string Target { get; }
    public string Name { get; }
    public string Uuid { get; }
    public string Created { get; }
    public string Source { get; }
    public string Expires { get; }
    public string Reason { get; }
    public string TypeText { get; }
    public string ReasonText { get; }
    public string IdentityText { get; }
    public string CreatedText { get; }
}
