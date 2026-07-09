using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>
/// A reusable responsive content container for pages.
/// It centers page content and limits the maximum width so wide screens don't stretch the UI.
/// </summary>
public sealed partial class SafeAreaContainer : ContentControl {
    public static readonly DependencyProperty MaxContentWidthProperty =
        DependencyProperty.Register(nameof(MaxContentWidth), typeof(double),
                                    typeof(SafeAreaContainer),
                                    new PropertyMetadata(1040.0));

    public SafeAreaContainer() {
        this.InitializeComponent();
    }

    public double MaxContentWidth {
        get => (double)GetValue(MaxContentWidthProperty);
        set => SetValue(MaxContentWidthProperty, value);
    }
}
