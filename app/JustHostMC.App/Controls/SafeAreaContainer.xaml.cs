using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Markup;

namespace JustHostMC.App.Controls;

/// <summary>
/// A container that constrains its content width to a maximum safe area (e.g., 1440px),
/// centering the content when the window is larger, and acting fluid when smaller.
/// </summary>
[ContentProperty(Name = nameof(InnerContent))]
public sealed partial class SafeAreaContainer : UserControl
{
    public static readonly DependencyProperty InnerContentProperty =
        DependencyProperty.Register(nameof(InnerContent), typeof(object),
                                    typeof(SafeAreaContainer),
                                    new PropertyMetadata(null));

    public SafeAreaContainer()
    {
        this.InitializeComponent();
    }

    public object InnerContent
    {
        get => GetValue(InnerContentProperty);
        set => SetValue(InnerContentProperty, value);
    }
}
