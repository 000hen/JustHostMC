using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls.Server;

public sealed partial class ServerPerformancePanel : UserControl {
    private readonly ILocalizer _localizer = new LocalizationService();

    public static readonly DependencyProperty MetricsProperty =
        DependencyProperty.Register(
            nameof(Metrics), typeof(MetricsViewModel),
            typeof(ServerPerformancePanel),
            new PropertyMetadata(null, OnMetricsChanged));

    public MetricsViewModel Metrics {
        get => (MetricsViewModel)GetValue(MetricsProperty);
        set => SetValue(MetricsProperty, value);
    }

    public ServerPerformancePanel() {
        InitializeComponent();
    }

    private string PerformanceTitle() =>
        _localizer.Get("ServerSectionPerformance/Text");

    private string PerformanceDescription() =>
        _localizer.Get("ServerSectionPerformanceHint/Text");

    private static void OnMetricsChanged(DependencyObject d,
                                         DependencyPropertyChangedEventArgs e) {
        var panel = (ServerPerformancePanel)d;
        panel.Bindings.Update();
    }
}
