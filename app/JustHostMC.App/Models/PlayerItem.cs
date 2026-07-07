using Microsoft.UI.Xaml.Media.Imaging;

namespace JustHostMC.App.Models;

/// <summary>Bindable roster item with the Minecraft skin head image.</summary>
public sealed class PlayerItem {
    public PlayerItem(string name, string uuid = "") {
        Name      = name;
        Uuid      = uuid;
        HeadImage = new BitmapImage(new Uri(
            $"https://mc-heads.net/avatar/{Uri.EscapeDataString(name)}/32"));
    }

    public string Name { get; }
    public string Uuid { get; }

    public BitmapImage HeadImage { get; }

    public override string ToString() => Name;
}
