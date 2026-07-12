using JustHostMC.App.Models;
using JustHostMC.App.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Shared card for provider and automation script entries.</summary>
public sealed partial class ScriptEntryCard : UserControl {
    public static readonly DependencyProperty ItemProperty =
        DependencyProperty.Register(nameof(Item), typeof(ScriptEntryItem),
                                    typeof(ScriptEntryCard),
                                    new PropertyMetadata(null));

    private readonly ILocalizer _localizer = new LocalizationService();

    public ScriptEntryCard() => InitializeComponent();

    public event RoutedEventHandler? RemoveClick;

    public event RoutedEventHandler? EnabledToggled;

    public ScriptEntryItem? Item {
        get => (ScriptEntryItem?)GetValue(ItemProperty);
        set => SetValue(ItemProperty, value);
    }

    public bool ScriptEnabled => EnabledToggle.IsOn;

    private void OnRemoveFlyoutOpening(object sender, object e) {
        if (Item is not null) {
            RemoveConfirmText.Text = _localizer.Get("Scripts.RemoveConfirmBody",
                                                    ("name", Item.Name));
            RemoveConfirmButton.Content =
                _localizer.Get("Scripts.RemoveConfirmPrimary");
        }
    }

    private void OnRemoveConfirmClick(object sender, RoutedEventArgs e) {
        RemoveFlyout.Hide();
        RemoveClick?.Invoke(this, e);
    }

    private void OnEnabledToggled(object sender, RoutedEventArgs e) =>
        EnabledToggled?.Invoke(this, e);
}
