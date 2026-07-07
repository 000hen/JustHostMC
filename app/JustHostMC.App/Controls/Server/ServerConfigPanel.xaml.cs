using JustHostMC.App.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls.Server;

public sealed partial class ServerConfigPanel : UserControl {
    public static readonly DependencyProperty ConfigProperty = DependencyProperty.Register(
        nameof(Config),
        typeof(ServerConfigViewModel),
        typeof(ServerConfigPanel),
        new PropertyMetadata(null, OnConfigChanged));

    public ServerConfigViewModel Config {
        get => (ServerConfigViewModel)GetValue(ConfigProperty);
        set => SetValue(ConfigProperty, value);
    }

    public ServerConfigPanel() {
        InitializeComponent();
    }

    private static void OnConfigChanged(DependencyObject d, DependencyPropertyChangedEventArgs e) {
        var panel = (ServerConfigPanel)d;
        panel.Bindings.Update();
    }

    private async void OnSaveModifiedConfigClick(object sender, RoutedEventArgs e) {
        if (Config != null) {
            await Config.SaveModifiedAsync();
        }
    }

    private void OnDiscardConfigChangesClick(object sender, RoutedEventArgs e) {
        Config?.DiscardChanges();
    }
}
