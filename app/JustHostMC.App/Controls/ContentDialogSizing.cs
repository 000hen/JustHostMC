using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Applies the app's responsive standard and wide widths to a native ContentDialog.</summary>
public static class ContentDialogSizing
{
    private const double StandardWidth = 720;
    private const double WideWidth = 960;
    private const double WindowInset = 96;
    private const double MinimumAvailableWidth = 320;
    private const double StandardMinWidth = 560;
    private const double WideMinWidth = 720;

    public static void Apply(ContentDialog dialog, bool useWideLayout = false)
    {
        void ApplySizing()
        {
            var targetWidth = useWideLayout ? WideWidth : StandardWidth;
            var targetMinWidth = useWideLayout ? WideMinWidth : StandardMinWidth;
            var availableWidth = GetAvailableWidth(dialog.XamlRoot, targetWidth);
            var dialogWidth = Math.Min(targetWidth, availableWidth);
            var minWidth = Math.Min(targetMinWidth, dialogWidth);

            dialog.Resources["ContentDialogMinWidth"] = minWidth;
            dialog.Resources["ContentDialogMaxWidth"] = dialogWidth;
            dialog.Resources["ContentDialogThemeMinWidth"] = minWidth;
            dialog.Resources["ContentDialogThemeMaxWidth"] = dialogWidth;
        }

        dialog.Loaded += (_, _) => ApplySizing();
        dialog.SizeChanged += (_, _) => ApplySizing();
        ApplySizing();
    }

    private static double GetAvailableWidth(XamlRoot? root, double fallbackWidth)
    {
        if (root is null || root.Size.Width <= 0)
            return fallbackWidth;

        return Math.Max(MinimumAvailableWidth, root.Size.Width - WindowInset);
    }
}
