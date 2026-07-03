using McManager.Grpc;
using Microsoft.UI.Composition;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Hosting;

namespace JustHostMC.App.Controls;

/// <summary>Adds the transitional-state blink animation to an icon-compatible status dot.</summary>
public static class StatusDotBehavior
{
    public static readonly DependencyProperty StatusProperty =
        DependencyProperty.RegisterAttached(
            "Status",
            typeof(ServerStatus),
            typeof(StatusDotBehavior),
            new PropertyMetadata(ServerStatus.Stopped, OnStatusChanged));

    public static ServerStatus GetStatus(DependencyObject element)
        => (ServerStatus)element.GetValue(StatusProperty);

    public static void SetStatus(DependencyObject element, ServerStatus value)
        => element.SetValue(StatusProperty, value);

    private static void OnStatusChanged(DependencyObject sender, DependencyPropertyChangedEventArgs args)
    {
        if (sender is not UIElement element)
            return;

        var visual = ElementCompositionPreview.GetElementVisual(element);
        visual.StopAnimation("Opacity");
        visual.Opacity = 1;

        var status = (ServerStatus)args.NewValue;
        if (status is not (ServerStatus.Starting or ServerStatus.Stopping or ServerStatus.Installing))
            return;

        var blink = visual.Compositor.CreateScalarKeyFrameAnimation();
        blink.InsertKeyFrame(0, 1);
        blink.InsertKeyFrame(1, 0.2f);
        blink.Duration = TimeSpan.FromMilliseconds(650);
        blink.Direction = AnimationDirection.Alternate;
        blink.IterationBehavior = AnimationIterationBehavior.Forever;
        visual.StartAnimation("Opacity", blink);
    }
}
