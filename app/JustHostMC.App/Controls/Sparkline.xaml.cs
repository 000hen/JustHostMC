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

    public Sparkline() => InitializeComponent();

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
        if (e.OldValue is INotifyCollectionChanged oldObservable)
            oldObservable.CollectionChanged -= self.OnSeriesChanged;
        if (e.NewValue is INotifyCollectionChanged newObservable)
            newObservable.CollectionChanged += self.OnSeriesChanged;
        self.Redraw();
    }

    private static void OnStrokeChanged(DependencyObject d,
                                        DependencyPropertyChangedEventArgs e) =>
        ((Sparkline)d).Line.Stroke = e.NewValue as Brush;

    private void OnSeriesChanged(
        object? sender, NotifyCollectionChangedEventArgs e) => Redraw();

    private void OnSizeChanged(object sender,
                               SizeChangedEventArgs e) => Redraw();

    private void Redraw() {
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
