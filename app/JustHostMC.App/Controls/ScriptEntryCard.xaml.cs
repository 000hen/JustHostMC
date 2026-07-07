using JustHostMC.App.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Shared card for provider and automation script entries.</summary>
public sealed partial class ScriptEntryCard : UserControl {
    public static readonly DependencyProperty ItemProperty =
        DependencyProperty.Register(nameof(Item), typeof(ScriptEntryItem),
                                    typeof(ScriptEntryCard),
                                    new PropertyMetadata(null));

    public ScriptEntryCard() => InitializeComponent();

    public event RoutedEventHandler? RemoveClick;

    public event RoutedEventHandler? EnabledToggled;

    public ScriptEntryItem? Item {
        get => (ScriptEntryItem?)GetValue(ItemProperty);
        set => SetValue(ItemProperty, value);
    }

    public bool ScriptEnabled => EnabledToggle.IsOn;

    private void OnRemoveClick(object sender,
                               RoutedEventArgs e) => RemoveClick?.Invoke(this,
                                                                         e);

    private void OnEnabledToggled(object sender, RoutedEventArgs e) =>
        EnabledToggled?.Invoke(this, e);
}
