using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>
/// Shared responsive shell for pages that show primary content plus custom filters.
/// The filter implementation stays with the consuming page; this control owns the
/// wide filter rail and compact filter flyout behavior.
/// </summary>
public sealed partial class AdaptiveFilterLayout : UserControl
{
    public static readonly DependencyProperty HeaderContentProperty = DependencyProperty.Register(
        nameof(HeaderContent),
        typeof(object),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(null));

    public static readonly DependencyProperty MainContentProperty = DependencyProperty.Register(
        nameof(MainContent),
        typeof(object),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(null));

    public static readonly DependencyProperty FooterContentProperty = DependencyProperty.Register(
        nameof(FooterContent),
        typeof(object),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(null));

    public static readonly DependencyProperty WideFilterContentProperty = DependencyProperty.Register(
        nameof(WideFilterContent),
        typeof(object),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(null));

    public static readonly DependencyProperty CompactFilterContentProperty = DependencyProperty.Register(
        nameof(CompactFilterContent),
        typeof(object),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(null));

    public static readonly DependencyProperty CompactPaddingProperty = DependencyProperty.Register(
        nameof(CompactPadding),
        typeof(Thickness),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(new Thickness(12, 8, 12, 0), OnLayoutPropertyChanged));

    public static readonly DependencyProperty WidePaddingProperty = DependencyProperty.Register(
        nameof(WidePadding),
        typeof(Thickness),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(new Thickness(24, 16, 24, 0), OnLayoutPropertyChanged));

    public static readonly DependencyProperty RowSpacingProperty = DependencyProperty.Register(
        nameof(RowSpacing),
        typeof(double),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(12d));

    public static readonly DependencyProperty HeaderSpacingProperty = DependencyProperty.Register(
        nameof(HeaderSpacing),
        typeof(double),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(12d));

    public static readonly DependencyProperty CompactColumnSpacingProperty = DependencyProperty.Register(
        nameof(CompactColumnSpacing),
        typeof(double),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(0d, OnLayoutPropertyChanged));

    public static readonly DependencyProperty WideColumnSpacingProperty = DependencyProperty.Register(
        nameof(WideColumnSpacing),
        typeof(double),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(20d, OnLayoutPropertyChanged));

    public static readonly DependencyProperty WideMinWidthProperty = DependencyProperty.Register(
        nameof(WideMinWidth),
        typeof(double),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(1050d, OnLayoutPropertyChanged));

    public static readonly DependencyProperty WideFilterWidthProperty = DependencyProperty.Register(
        nameof(WideFilterWidth),
        typeof(double),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(292d, OnLayoutPropertyChanged));

    public static readonly DependencyProperty CompactFilterMaxHeightProperty = DependencyProperty.Register(
        nameof(CompactFilterMaxHeight),
        typeof(double),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(620d));

    public static readonly DependencyProperty FilterRailCornerRadiusProperty = DependencyProperty.Register(
        nameof(FilterRailCornerRadius),
        typeof(CornerRadius),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(new CornerRadius(10)));

    public static readonly DependencyProperty FilterRailIncludesHeaderProperty = DependencyProperty.Register(
        nameof(FilterRailIncludesHeader),
        typeof(bool),
        typeof(AdaptiveFilterLayout),
        new PropertyMetadata(false, OnLayoutPropertyChanged));

    public AdaptiveFilterLayout()
    {
        InitializeComponent();
        SizeChanged += OnSizeChanged;
        ApplyLayout();
    }

    public event EventHandler<object>? CompactFilterOpening;

    public object? HeaderContent
    {
        get => GetValue(HeaderContentProperty);
        set => SetValue(HeaderContentProperty, value);
    }

    public object? MainContent
    {
        get => GetValue(MainContentProperty);
        set => SetValue(MainContentProperty, value);
    }

    public object? FooterContent
    {
        get => GetValue(FooterContentProperty);
        set => SetValue(FooterContentProperty, value);
    }

    public object? WideFilterContent
    {
        get => GetValue(WideFilterContentProperty);
        set => SetValue(WideFilterContentProperty, value);
    }

    public object? CompactFilterContent
    {
        get => GetValue(CompactFilterContentProperty);
        set => SetValue(CompactFilterContentProperty, value);
    }

    public Thickness CompactPadding
    {
        get => (Thickness)GetValue(CompactPaddingProperty);
        set => SetValue(CompactPaddingProperty, value);
    }

    public Thickness WidePadding
    {
        get => (Thickness)GetValue(WidePaddingProperty);
        set => SetValue(WidePaddingProperty, value);
    }

    public double RowSpacing
    {
        get => (double)GetValue(RowSpacingProperty);
        set => SetValue(RowSpacingProperty, value);
    }

    public double HeaderSpacing
    {
        get => (double)GetValue(HeaderSpacingProperty);
        set => SetValue(HeaderSpacingProperty, value);
    }

    public double CompactColumnSpacing
    {
        get => (double)GetValue(CompactColumnSpacingProperty);
        set => SetValue(CompactColumnSpacingProperty, value);
    }

    public double WideColumnSpacing
    {
        get => (double)GetValue(WideColumnSpacingProperty);
        set => SetValue(WideColumnSpacingProperty, value);
    }

    public double WideMinWidth
    {
        get => (double)GetValue(WideMinWidthProperty);
        set => SetValue(WideMinWidthProperty, value);
    }

    public double WideFilterWidth
    {
        get => (double)GetValue(WideFilterWidthProperty);
        set => SetValue(WideFilterWidthProperty, value);
    }

    public double CompactFilterMaxHeight
    {
        get => (double)GetValue(CompactFilterMaxHeightProperty);
        set => SetValue(CompactFilterMaxHeightProperty, value);
    }

    public CornerRadius FilterRailCornerRadius
    {
        get => (CornerRadius)GetValue(FilterRailCornerRadiusProperty);
        set => SetValue(FilterRailCornerRadiusProperty, value);
    }

    public bool FilterRailIncludesHeader
    {
        get => (bool)GetValue(FilterRailIncludesHeaderProperty);
        set => SetValue(FilterRailIncludesHeaderProperty, value);
    }

    private static void OnLayoutPropertyChanged(DependencyObject d, DependencyPropertyChangedEventArgs e)
    {
        if (d is AdaptiveFilterLayout layout)
            layout.ApplyLayout();
    }

    private void OnSizeChanged(object sender, SizeChangedEventArgs e) => ApplyLayout();

    private void OnCompactFilterOpening(object sender, object e)
    {
        // Flyout content is hosted outside the normal visual tree, so ElementName
        // bindings from inside the popup can resolve too early or not at all.
        // Assign the compact filter body explicitly when the popup opens.
        CompactFilterScrollViewer.MaxHeight = CompactFilterMaxHeight;
        CompactFilterPresenter.Content = CompactFilterContent;
        CompactFilterOpening?.Invoke(this, e);
    }

    private void ApplyLayout()
    {
        if (RootLayout is null)
            return;

        var isWide = ActualWidth >= WideMinWidth;
        RootLayout.Padding = isWide ? WidePadding : CompactPadding;
        RootLayout.ColumnSpacing = isWide ? WideColumnSpacing : CompactColumnSpacing;
        FilterColumn.Width = new GridLength(isWide ? WideFilterWidth : 0);
        FilterRail.Visibility = isWide ? Visibility.Visible : Visibility.Collapsed;
        CompactFilterButton.Visibility = isWide ? Visibility.Collapsed : Visibility.Visible;

        Grid.SetRow(FilterRail, FilterRailIncludesHeader ? 0 : 1);
        Grid.SetRowSpan(FilterRail, FilterRailIncludesHeader ? 2 : 1);
    }
}
