using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>An expandable script section with standard folder and add actions.</summary>
public sealed partial class ScriptSectionExpander : UserControl
{
    public static readonly DependencyProperty HeaderContentProperty =
        Register<object?>(nameof(HeaderContent), null);

    public static readonly DependencyProperty SectionContentProperty =
        Register<object?>(nameof(SectionContent), null);

    public static readonly DependencyProperty IsExpandedProperty =
        Register<bool>(nameof(IsExpanded), true);

    public ScriptSectionExpander() => InitializeComponent();

    public event RoutedEventHandler? ShowInFolderClick;

    public event RoutedEventHandler? AddScriptsClick;

    public object? HeaderContent
    {
        get => GetValue(HeaderContentProperty);
        set => SetValue(HeaderContentProperty, value);
    }

    public object? SectionContent
    {
        get => GetValue(SectionContentProperty);
        set => SetValue(SectionContentProperty, value);
    }

    public bool IsExpanded
    {
        get => (bool)GetValue(IsExpandedProperty);
        set => SetValue(IsExpandedProperty, value);
    }

    private void OnShowInFolderClick(object sender, RoutedEventArgs e) =>
        ShowInFolderClick?.Invoke(this, e);

    private void OnAddScriptsClick(object sender, RoutedEventArgs e) =>
        AddScriptsClick?.Invoke(this, e);

    private static DependencyProperty Register<T>(string name, T defaultValue) =>
        DependencyProperty.Register(name, typeof(T), typeof(ScriptSectionExpander),
            new PropertyMetadata(defaultValue));
}
