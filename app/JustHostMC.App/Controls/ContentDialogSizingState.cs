namespace JustHostMC.App.Controls;

internal sealed class ContentDialogSizingState {
    private const double StandardWidth    = 720;
    private const double WideWidth        = 960;
    private const double StandardMinWidth = 560;
    private const double WideMinWidth     = 720;

    public bool UseWideLayout { get; set; }

    public double TargetWidth => UseWideLayout ? WideWidth : StandardWidth;

    public (double Width, double MinWidth) Calculate(double availableWidth) {
        var width = Math.Min(TargetWidth, availableWidth);
        var targetMinWidth = UseWideLayout ? WideMinWidth : StandardMinWidth;
        return (width, Math.Min(targetMinWidth, width));
    }
}
