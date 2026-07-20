using System.Collections;
using System.Collections.Specialized;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Windows.Foundation;

namespace JustHostMC.App.Controls;

/// <summary>A minimal line chart: plots a bounded series of 0..1 values as a
/// polyline. Dependency-free (no charting package). Bind <see cref="Values"/>
/// to an ObservableCollection&lt;double&gt; of normalized samples.</summary>
public sealed partial class Sparkline : UserControl {
    public static readonly DependencyProperty ValuesProperty =
        DependencyProperty.Register(
            nameof(Values), typeof(object), typeof(Sparkline),
            new PropertyMetadata(null, OnValuesChanged));

    public static readonly DependencyProperty StrokeBrushProperty =
        DependencyProperty.Register(
            nameof(StrokeBrush), typeof(Brush), typeof(Sparkline),
            new PropertyMetadata(null, OnStrokeChanged));

    private INotifyCollectionChanged? _observableValues;
    private bool _seriesSubscriptionAttached;
    private bool _isLoaded;

    public Sparkline() {
        InitializeComponent();
        Loaded += OnLoaded;
        Unloaded += OnUnloaded;
    }

    public object? Values {
        get => GetValue(ValuesProperty);
        set => SetValue(ValuesProperty, value);
    }

    public Brush? StrokeBrush {
        get => (Brush?)GetValue(StrokeBrushProperty);
        set => SetValue(StrokeBrushProperty, value);
    }

    private static void OnValuesChanged(DependencyObject d,
                                        DependencyPropertyChangedEventArgs e) {
        var self = (Sparkline)d;
        self.DetachSeries();
        self._observableValues = e.NewValue as INotifyCollectionChanged;
        if (self._isLoaded) {
            self.AttachSeries();
            self.Redraw();
        }
    }

    private static void OnStrokeChanged(DependencyObject d,
                                        DependencyPropertyChangedEventArgs e) =>
        ((Sparkline)d).Line.Stroke = e.NewValue as Brush;

    private void OnSeriesChanged(
        object? sender, NotifyCollectionChangedEventArgs e) => Redraw();

    private void OnSizeChanged(object sender,
                               SizeChangedEventArgs e) => Redraw();

    private void OnLoaded(object sender, RoutedEventArgs e) {
        _isLoaded = true;
        AttachSeries();
        Redraw();
    }

    private void OnUnloaded(object sender, RoutedEventArgs e) {
        _isLoaded = false;
        DetachSeries();
    }

    private void AttachSeries() {
        if (_seriesSubscriptionAttached || _observableValues is null)
            return;

        _observableValues.CollectionChanged += OnSeriesChanged;
        _seriesSubscriptionAttached = true;
    }

    private void DetachSeries() {
        if (!_seriesSubscriptionAttached)
            return;

        if (_observableValues is not null)
            _observableValues.CollectionChanged -= OnSeriesChanged;
        _seriesSubscriptionAttached = false;
    }

    private void Redraw() {
        if (!_isLoaded)
            return;

        Line.Points.Clear();

        double width  = ActualWidth;
        double height = ActualHeight;
        if (width <= 0 || height <= 0 || Values is not IEnumerable series)
            return;

        // O(1) count for ObservableCollection (ICollection) instead of
        // enumerating twice.
        int count;
        if (series is System.Collections.ICollection col)
            count = col.Count;
        else {
            count = 0;
            foreach (var _ in series) count++;
        }
        if (count < 2)
            return;

        var points          = new PointCollection();
        const double pad    = 2;
        double usableHeight = height - 2 * pad;
        double step         = width / (count - 1);
        int i               = 0;
        foreach (var value in series) {
            double v = value is double dv ? dv : 0;
            if (v < 0)
                v = 0;
            if (v > 1)
                v = 1;
            double x = i * step;
            double y = pad + (1 - v) * usableHeight;
            points.Add(new Point(x, y));
            i++;
        }
        Line.Points = points;
    }
}
