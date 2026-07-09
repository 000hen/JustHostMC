using System.Collections;
using System.Collections.Specialized;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>An expandable script section with standard folder and add
/// actions.</summary>
public sealed partial class ScriptSectionExpander : UserControl {
    public static readonly DependencyProperty HeaderContentProperty =
        Register<object?>(nameof(HeaderContent), null);

    public static readonly DependencyProperty HeaderAdditionalButtonsProperty =
        Register<object?>(nameof(HeaderAdditionalButtons), null);

    public static readonly DependencyProperty SectionContentProperty =
        Register<object?>(nameof(SectionContent), null);

    public static readonly DependencyProperty IsExpandedProperty =
        Register<bool>(nameof(IsExpanded), true);

    public static readonly DependencyProperty ItemsSourceProperty =
        DependencyProperty.Register(
            nameof(ItemsSource), typeof(IEnumerable),
            typeof(ScriptSectionExpander),
            new PropertyMetadata(null, OnItemsSourceChanged));

    private static void OnItemsSourceChanged(
        DependencyObject d, DependencyPropertyChangedEventArgs e) {
        var self = (ScriptSectionExpander)d;
        self.UpdateIsEmpty();

        if (e.OldValue is INotifyCollectionChanged oldCollection) {
            oldCollection.CollectionChanged -= self.OnCollectionChanged;
        }
        if (e.NewValue is INotifyCollectionChanged newCollection) {
            newCollection.CollectionChanged += self.OnCollectionChanged;
        }
    }

    private void OnCollectionChanged(object? sender,
                                     NotifyCollectionChangedEventArgs e) {
        if (DispatcherQueue.HasThreadAccess) {
            UpdateIsEmpty();
        } else {
            DispatcherQueue.TryEnqueue(UpdateIsEmpty);
        }
    }

    private void UpdateIsEmpty() {
        var items   = ItemsSource;
        var isEmpty = items == null || !items.Cast<object>().Any();
        SetValue(IsEmptyProperty, isEmpty);
    }

    public static readonly DependencyProperty IsEmptyProperty =
        Register<bool>(nameof(IsEmpty), true);

    public ScriptSectionExpander() => InitializeComponent();

    public event RoutedEventHandler? ShowInFolderClick;
    public event RoutedEventHandler? AddScriptsClick;
    public event RoutedEventHandler? ScriptToggled;
    public event RoutedEventHandler? ScriptRemoved;

    public object? HeaderContent {
        get => GetValue(HeaderContentProperty);
        set => SetValue(HeaderContentProperty, value);
    }

    public object? HeaderAdditionalButtons {
        get => GetValue(HeaderAdditionalButtonsProperty);
        set => SetValue(HeaderAdditionalButtonsProperty, value);
    }

    public IEnumerable ItemsSource {
        get => (IEnumerable)GetValue(ItemsSourceProperty);
        set => SetValue(ItemsSourceProperty, value);
    }

    public object? SectionContent {
        get => GetValue(SectionContentProperty);
        set => SetValue(SectionContentProperty, value);
    }

    public bool IsExpanded {
        get => (bool)GetValue(IsExpandedProperty);
        set => SetValue(IsExpandedProperty, value);
    }

    public bool IsEmpty {
        get => (bool)GetValue(IsEmptyProperty);
        set => SetValue(IsEmptyProperty, value);
    }

    private void OnShowInFolderClick(object sender, RoutedEventArgs e) =>
        ShowInFolderClick?.Invoke(this, e);

    private void OnAddScriptsClick(object sender, RoutedEventArgs e) =>
        AddScriptsClick?.Invoke(this, e);

    private void OnScriptToggled(object sender, RoutedEventArgs e) =>
        ScriptToggled?.Invoke(sender, e);

    private void OnScriptRemoved(object sender, RoutedEventArgs e) =>
        ScriptRemoved?.Invoke(sender, e);

    private static DependencyProperty Register<T>(string name,
                                                  T defaultValue) =>
        DependencyProperty.Register(name, typeof(T),
                                    typeof(ScriptSectionExpander),
                                    new PropertyMetadata(defaultValue));
}
