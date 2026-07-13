using System.Runtime.CompilerServices;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Applies the app's responsive standard and wide widths to a native
/// ContentDialog.</summary>
public static class ContentDialogSizing {
    private const double WindowInset           = 96;
    private const double MinimumAvailableWidth = 320;
    private static readonly ConditionalWeakTable<ContentDialog, SizingState>
        States = new();

    public static void Apply(ContentDialog dialog, bool useWideLayout = false) {
        var state = States.GetValue(dialog, static dialog => {
            var state = new SizingState();
            dialog.Loaded += state.OnLoaded;
            dialog.SizeChanged += state.OnSizeChanged;
            return state;
        });
        state.UseWideLayout = useWideLayout;
        state.Apply(dialog);
    }

    private sealed class SizingState {
        private readonly ContentDialogSizingState _layout = new();

        public bool UseWideLayout {
            get => _layout.UseWideLayout;
            set => _layout.UseWideLayout = value;
        }

        public void Apply(ContentDialog dialog) {
            var availableWidth =
                GetAvailableWidth(dialog.XamlRoot, _layout.TargetWidth);
            var (dialogWidth, minWidth) = _layout.Calculate(availableWidth);

            dialog.Resources["ContentDialogMinWidth"]      = minWidth;
            dialog.Resources["ContentDialogMaxWidth"]      = dialogWidth;
            dialog.Resources["ContentDialogThemeMinWidth"] = minWidth;
            dialog.Resources["ContentDialogThemeMaxWidth"] = dialogWidth;
        }

        public void OnLoaded(object sender, RoutedEventArgs args) =>
            Apply((ContentDialog)sender);

        public void OnSizeChanged(object sender, SizeChangedEventArgs args) =>
            Apply((ContentDialog)sender);
    }

    private static double GetAvailableWidth(XamlRoot? root,
                                            double fallbackWidth) {
        if (root is null || root.Size.Width <= 0)
            return fallbackWidth;

        return Math.Max(MinimumAvailableWidth, root.Size.Width - WindowInset);
    }
}
