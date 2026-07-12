using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls.Server;

public sealed partial class ServerConfigPanel : UserControl {
    private readonly ILocalizer _localizer = new LocalizationService();

    public static readonly DependencyProperty ConfigProperty =
        DependencyProperty.Register(
            nameof(Config), typeof(ServerConfigViewModel),
            typeof(ServerConfigPanel),
            new PropertyMetadata(null, OnConfigChanged));

    public ServerConfigViewModel Config {
        get => (ServerConfigViewModel)GetValue(ConfigProperty);
        set => SetValue(ConfigProperty, value);
    }

    public ServerConfigPanel() {
        InitializeComponent();
    }

    private string ConfigTitle() => _localizer.Get("ServerSectionConfig.Text");

    private string ConfigDescription(bool canModify) =>
        canModify ? _localizer.Get("ServerSectionConfigHint.Text")
                  : _localizer.Get("ConfigStoppedHint.Text");

    private static void OnConfigChanged(DependencyObject d,
                                        DependencyPropertyChangedEventArgs e) {
        var panel = (ServerConfigPanel)d;
        panel.Bindings.Update();
    }

    private async void OnSaveModifiedConfigClick(object sender,
                                                 RoutedEventArgs e) {
        if (Config != null) {
            await Config.SaveModifiedAsync();
        }
    }

    private void OnDiscardConfigChangesClick(object sender, RoutedEventArgs e) {
        Config?.DiscardChanges();
    }
}
