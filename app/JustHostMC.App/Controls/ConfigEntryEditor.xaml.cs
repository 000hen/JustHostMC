using JustHostMC.App.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Displays and edits one server property or gamerule.</summary>
public sealed partial class ConfigEntryEditor : UserControl {
    public static readonly DependencyProperty EntryProperty =
        DependencyProperty.Register(nameof(Entry), typeof(ConfigEntryItem),
                                    typeof(ConfigEntryEditor),
                                    new PropertyMetadata(null));

    public ConfigEntryEditor() => InitializeComponent();

    public ConfigEntryItem? Entry {
        get => (ConfigEntryItem?)GetValue(EntryProperty);
        set => SetValue(EntryProperty, value);
    }
}
