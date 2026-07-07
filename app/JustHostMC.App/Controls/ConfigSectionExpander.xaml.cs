using System.Collections;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Hosts a titled, expandable collection of configuration
/// editors.</summary>
public sealed partial class ConfigSectionExpander : UserControl {
    public static readonly DependencyProperty TitleProperty =
        Register<string>(nameof(Title), "");
    public static readonly DependencyProperty DescriptionProperty =
        Register<string>(nameof(Description), "");
    public static readonly DependencyProperty ItemsSourceProperty =
        Register<IEnumerable?>(nameof(ItemsSource), null);
    public static readonly DependencyProperty MessageProperty =
        Register<string>(nameof(Message), "");
    public static readonly DependencyProperty HasMessageProperty =
        Register<bool>(nameof(HasMessage), false);
    public static readonly DependencyProperty IsExpandedProperty =
        Register<bool>(nameof(IsExpanded), false);

    public ConfigSectionExpander() => InitializeComponent();

    public string Title {
        get => (string)GetValue(TitleProperty);
        set => SetValue(TitleProperty, value);
    }

    public string Description {
        get => (string)GetValue(DescriptionProperty);
        set => SetValue(DescriptionProperty, value);
    }

    public IEnumerable? ItemsSource {
        get => (IEnumerable?)GetValue(ItemsSourceProperty);
        set => SetValue(ItemsSourceProperty, value);
    }

    public string Message {
        get => (string)GetValue(MessageProperty);
        set => SetValue(MessageProperty, value);
    }

    public bool HasMessage {
        get => (bool)GetValue(HasMessageProperty);
        set => SetValue(HasMessageProperty, value);
    }

    public bool IsExpanded {
        get => (bool)GetValue(IsExpandedProperty);
        set => SetValue(IsExpandedProperty, value);
    }

    private static DependencyProperty Register<T>(string name,
                                                  T defaultValue) =>
        DependencyProperty.Register(name, typeof(T),
                                    typeof(ConfigSectionExpander),
                                    new PropertyMetadata(defaultValue));
}
