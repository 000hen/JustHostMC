using JustHostMC.App.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls.Server;

public sealed partial class ServerPerformancePanel : UserControl {
    public static readonly DependencyProperty MetricsProperty = DependencyProperty.Register(
        nameof(Metrics),
        typeof(MetricsViewModel),
        typeof(ServerPerformancePanel),
        new PropertyMetadata(null, OnMetricsChanged));

    public MetricsViewModel Metrics {
        get => (MetricsViewModel)GetValue(MetricsProperty);
        set => SetValue(MetricsProperty, value);
    }

    public ServerPerformancePanel() {
        InitializeComponent();
    }

    private static void OnMetricsChanged(DependencyObject d, DependencyPropertyChangedEventArgs e) {
        var panel = (ServerPerformancePanel)d;
        panel.Bindings.Update();
    }
}
