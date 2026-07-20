using JustHostMC.App.Controls;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class ServerUpdateModpackDialog : ContentDialog {
    private readonly IReadOnlyList<ShopVersion> _versions;

    public ServerUpdateModpackDialog(string serverName,
                                     string currentVersionName,
                                     IReadOnlyList<ShopVersion> versions) {
        ServerName         = serverName;
        CurrentVersionName = currentVersionName;
        _versions          = versions;
        VersionNames       = versions.Select(DisplayName).ToArray();

        InitializeComponent();
        ContentDialogSizing.Apply(this);

        var hasVersions = versions.Count > 0;
        ChoicesPanel.Visibility =
            hasVersions ? Visibility.Visible : Visibility.Collapsed;
        NoVersionsText.Visibility =
            hasVersions ? Visibility.Collapsed : Visibility.Visible;
        if (!hasVersions)
            PrimaryButtonText = "";
    }

    public string ServerName { get; }
    public string CurrentVersionName { get; }
    public IReadOnlyList<string> VersionNames { get; }

    public ShopVersion? SelectedVersion =>
        VersionBox.SelectedIndex >= 0 &&
                VersionBox.SelectedIndex < _versions.Count
            ? _versions[VersionBox.SelectedIndex]
            : null;

    private static string DisplayName(ShopVersion version) =>
        version.Name.Length > 0? version.Name : version.VersionNumber;
}
