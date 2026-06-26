using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Windows.Foundation;

namespace JustHostMC.App.Controls;

/// <summary>Shared native WinUI ContentDialog with app-level width presets.</summary>
public class FluentContentDialog : ContentDialog
{
    private const double StandardWidth = 720;
    private const double WideWidth = 960;
    private const double WindowInset = 96;
    private const double MinimumAvailableWidth = 320;
    private const double StandardMinWidth = 560;
    private const double WideMinWidth = 720;

    private bool _useWideLayout;

    public FluentContentDialog()
    {
        Loaded += (_, _) => ApplyDialogSizing();
        SizeChanged += (_, _) => ApplyDialogSizing();
    }

    public bool UseWideLayout
    {
        get => _useWideLayout;
        set
        {
            _useWideLayout = value;
            ApplyDialogSizing();
        }
    }

    public new IAsyncOperation<ContentDialogResult> ShowAsync()
    {
        if (XamlRoot is null && App.Current.MainWindow?.Content is FrameworkElement root)
            XamlRoot = root.XamlRoot;

        ApplyDialogSizing();
        return base.ShowAsync();
    }

    private void ApplyDialogSizing()
    {
        var targetWidth = UseWideLayout ? WideWidth : StandardWidth;
        var targetMinWidth = UseWideLayout ? WideMinWidth : StandardMinWidth;
        var availableWidth = GetAvailableWidth(targetWidth);
        var dialogWidth = Math.Min(targetWidth, availableWidth);
        var minWidth = Math.Min(targetMinWidth, dialogWidth);

        Width = dialogWidth;
        MinWidth = minWidth;
        MaxWidth = dialogWidth;

        Resources["ContentDialogMinWidth"] = minWidth;
        Resources["ContentDialogMaxWidth"] = dialogWidth;
        Resources["ContentDialogThemeMinWidth"] = minWidth;
        Resources["ContentDialogThemeMaxWidth"] = dialogWidth;
    }

    private double GetAvailableWidth(double fallbackWidth)
    {
        var root = XamlRoot;
        if (root is null || root.Size.Width <= 0)
            return fallbackWidth;

        return Math.Max(MinimumAvailableWidth, root.Size.Width - WindowInset);
    }
}
