using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace JustHostMC.App.Controls;

/// <summary>A small colored dot showing a server's state: green running, gray stopped,
/// red crashed, and a blinking amber for transitional states (starting/stopping/installing).</summary>
public sealed partial class StatusDot : UserControl
{
    public static readonly DependencyProperty StatusProperty =
        DependencyProperty.Register(
            nameof(Status), typeof(ServerStatus), typeof(StatusDot),
            new PropertyMetadata(ServerStatus.Stopped, OnStatusChanged));

    public StatusDot() => InitializeComponent();

    public ServerStatus Status
    {
        get => (ServerStatus)GetValue(StatusProperty);
        set => SetValue(StatusProperty, value);
    }

    private static void OnStatusChanged(DependencyObject d, DependencyPropertyChangedEventArgs e)
        => ((StatusDot)d).UpdateVisual();

    private void OnLoaded(object sender, RoutedEventArgs e) => UpdateVisual();

    private void UpdateVisual()
    {
        Dot.Fill = BrushFor(Status);
        if (IsTransitional(Status))
        {
            BlinkStoryboard.Begin();
        }
        else
        {
            BlinkStoryboard.Stop();
            Dot.Opacity = 1;
        }
        AutomationProperties.SetName(this, Status.ToString());
    }

    private static bool IsTransitional(ServerStatus s) =>
        s is ServerStatus.Starting or ServerStatus.Stopping or ServerStatus.Installing;

    private static Brush BrushFor(ServerStatus s) => (Brush)Application.Current.Resources[s switch
    {
        ServerStatus.Running => "SystemFillColorSuccessBrush",
        ServerStatus.Crashed => "SystemFillColorCriticalBrush",
        ServerStatus.Starting or ServerStatus.Stopping or ServerStatus.Installing => "SystemFillColorCautionBrush",
        _ => "TextFillColorDisabledBrush",
    }];
}
