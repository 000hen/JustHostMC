using System.Collections;
using System.Collections.Specialized;
using JustHostMC.App.Services;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;

namespace JustHostMC.App.Controls;

/// <summary>
/// Virtualized log surface that follows new entries only while the user is at
/// the bottom. A compact jump button appears when older entries are visible.
/// </summary>
public sealed partial class LogViewer : UserControl {
    private const double BottomTolerance = 2;
    private const int MaxStableTailScrollAttempts = 120;

    public static readonly DependencyProperty ItemsSourceProperty =
        DependencyProperty.Register(
            nameof(ItemsSource), typeof(IEnumerable), typeof(LogViewer),
            new PropertyMetadata(null, OnItemsSourceChanged));

    public static readonly DependencyProperty ItemTemplateProperty =
        DependencyProperty.Register(
            nameof(ItemTemplate), typeof(DataTemplate), typeof(LogViewer),
            new PropertyMetadata(null, OnItemTemplateChanged));

    public static readonly DependencyProperty EmptyContentProperty =
        DependencyProperty.Register(
            nameof(EmptyContent), typeof(object), typeof(LogViewer),
            new PropertyMetadata(null, OnEmptyContentChanged));

    private INotifyCollectionChanged? _observableItems;
    private ScrollViewer? _scrollViewer;
    private ItemsStackPanel? _itemsPanel;
    private readonly DispatcherQueueTimer _tailScrollTimer;
    private bool _itemsSubscriptionAttached;
    private bool _scrollViewerSubscribed;
    private bool _isAtBottom = true;
    private bool _scrollStateUpdateScheduled;
    private bool _tailScrollActive;
    private int _stableTailScrollAttempts;
    private long _contentVersion;
    private long _lastAttemptedVersion = -1;
    private long _layoutVersion;
    private long _lastAttemptedLayoutVersion = -1;

    public LogViewer() {
        InitializeComponent();
        LogList.ItemTemplate = DefaultItemTemplate;
        AutomationProperties.SetName(
            JumpToLatestButton,
            new LocalizationService().Get("LogViewer_JumpToLatest"));
        LogList.AddHandler(
            PointerWheelChangedEvent,
            new PointerEventHandler(OnUserScrollInput), true);
        _tailScrollTimer             = DispatcherQueue.CreateTimer();
        _tailScrollTimer.Interval    = TimeSpan.FromMilliseconds(16);
        _tailScrollTimer.IsRepeating = true;
        _tailScrollTimer.Tick       += OnTailScrollTick;
        LogList.LayoutUpdated       += OnLogListLayoutUpdated;
        Loaded   += OnLoaded;
        Unloaded += OnUnloaded;
    }

    public IEnumerable? ItemsSource {
        get => (IEnumerable?)GetValue(ItemsSourceProperty);
        set => SetValue(ItemsSourceProperty, value);
    }

    public DataTemplate? ItemTemplate {
        get => (DataTemplate?)GetValue(ItemTemplateProperty);
        set => SetValue(ItemTemplateProperty, value);
    }

    public object? EmptyContent {
        get => GetValue(EmptyContentProperty);
        set => SetValue(EmptyContentProperty, value);
    }

    private DataTemplate DefaultItemTemplate =>
        (DataTemplate)Resources["DefaultLogLineTemplate"];

    private static void OnItemsSourceChanged(
        DependencyObject d, DependencyPropertyChangedEventArgs e) {
        var viewer = (LogViewer)d;
        viewer.DetachItemsSource();
        viewer._observableItems = e.NewValue as INotifyCollectionChanged;
        if (viewer.IsLoaded)
            viewer.AttachItemsSource();

        viewer._isAtBottom = true;
        viewer._tailScrollActive = false;
        viewer.UpdateVisualState();
        if (viewer.IsLoaded)
            viewer.RequestScrollToBottom();
    }

    private static void OnItemTemplateChanged(
        DependencyObject d, DependencyPropertyChangedEventArgs e) {
        var viewer = (LogViewer)d;
        viewer.LogList.ItemTemplate = e.NewValue as DataTemplate ??
                                      viewer.DefaultItemTemplate;
    }

    private static void OnEmptyContentChanged(
        DependencyObject d, DependencyPropertyChangedEventArgs e) =>
        ((LogViewer)d).UpdateVisualState();

    private void OnLoaded(object sender, RoutedEventArgs e) {
        AttachItemsSource();
        AttachScrollViewer();
        UpdateVisualState();
        RequestScrollToBottom();
    }

    private void OnUnloaded(object sender, RoutedEventArgs e) {
        StopTailScroll();
        DetachItemsSource();
        if (_scrollViewer is not null)
            _scrollViewer.ViewChanged -= OnScrollViewChanged;
        _scrollViewerSubscribed = false;
        _scrollViewer = null;
        _itemsPanel   = null;
    }

    private void AttachScrollViewer() {
        if (!IsLoaded)
            return;

        if (_scrollViewer is null) {
            _scrollViewer = FindDescendant<ScrollViewer>(LogList);
        }
        if (_itemsPanel is null) {
            _itemsPanel = FindDescendant<ItemsStackPanel>(LogList);
        }

        if (_scrollViewer is not null && !_scrollViewerSubscribed) {
            _scrollViewer.ViewChanged += OnScrollViewChanged;
            _scrollViewerSubscribed = true;
        }

        UpdateScrollState();
        UpdateItemsPanelMode();
        if (_scrollViewer is null) {
            DispatcherQueue.TryEnqueue(DispatcherQueuePriority.Low,
                                       AttachScrollViewer);
        }
    }

    private void AttachItemsSource() {
        if (_itemsSubscriptionAttached || _observableItems is null)
            return;

        _observableItems.CollectionChanged += OnCollectionChanged;
        _itemsSubscriptionAttached = true;
    }

    private void DetachItemsSource() {
        if (!_itemsSubscriptionAttached || _observableItems is null)
            return;

        _observableItems.CollectionChanged -= OnCollectionChanged;
        _itemsSubscriptionAttached = false;
    }

    private void OnCollectionChanged(object? sender,
                                     NotifyCollectionChangedEventArgs e) {
        if (!DispatcherQueue.HasThreadAccess) {
            var followTail = _isAtBottom;
            DispatcherQueue.TryEnqueue(
                () => HandleCollectionChanged(followTail));
            return;
        }

        HandleCollectionChanged(_isAtBottom);
    }

    private void HandleCollectionChanged(bool followTail) {
        _contentVersion++;
        followTail |= _tailScrollActive;
        UpdateVisualState();
        if (!IsLoaded)
            return;

        if (followTail)
            RequestScrollToBottom();
        else
            QueueScrollStateUpdate();
    }

    private void OnScrollViewChanged(object? sender,
                                     ScrollViewerViewChangedEventArgs e) {
        if (e.IsIntermediate && _tailScrollActive)
            StopTailScroll();
        UpdateScrollState();
    }

    private void OnLogListLayoutUpdated(object? sender, object e) =>
        _layoutVersion++;

    private void UpdateScrollState() {
        if (_scrollViewer is null)
            return;

        _isAtBottom = _scrollViewer.ScrollableHeight <= BottomTolerance ||
                      _scrollViewer.VerticalOffset >=
                      _scrollViewer.ScrollableHeight - BottomTolerance;
        UpdateItemsPanelMode();
        UpdateVisualState();
    }

    private void RequestScrollToBottom() {
        if (!IsLoaded || !HasItems())
            return;

        if (!_tailScrollActive) {
            _stableTailScrollAttempts = 0;
            _lastAttemptedVersion     = -1;
            _lastAttemptedLayoutVersion = -1;
        }
        _tailScrollActive = true;
        UpdateItemsPanelMode();
        UpdateVisualState();
        if (!_tailScrollTimer.IsRunning)
            _tailScrollTimer.Start();
    }

    private void OnTailScrollTick(DispatcherQueueTimer sender, object args) {
        if (!_tailScrollActive || !IsLoaded) {
            StopTailScroll();
            return;
        }

        if (!TryGetLastItem(out var lastItem)) {
            _isAtBottom = true;
            StopTailScroll();
            return;
        }

        AttachScrollViewer();
        UpdateScrollState();
        var laidOutSinceAttempt =
            _layoutVersion > _lastAttemptedLayoutVersion;
        if (_isAtBottom && _lastAttemptedVersion == _contentVersion &&
            laidOutSinceAttempt) {
            StopTailScroll();
            return;
        }

        if (_lastAttemptedVersion == _contentVersion) {
            _stableTailScrollAttempts++;
            if (_stableTailScrollAttempts >= MaxStableTailScrollAttempts) {
                StopTailScroll();
                return;
            }
        } else {
            _stableTailScrollAttempts = 0;
        }

        _lastAttemptedVersion = _contentVersion;
        _lastAttemptedLayoutVersion = _layoutVersion;
        LogList.ScrollIntoView(lastItem);
        _scrollViewer?.ChangeView(null, _scrollViewer.ScrollableHeight, null,
                                  true);
    }

    private void StopTailScroll() {
        _tailScrollTimer.Stop();
        _tailScrollActive = false;
        UpdateItemsPanelMode();
        UpdateVisualState();
    }

    private void UpdateItemsPanelMode() {
        if (_itemsPanel is null)
            return;

        _itemsPanel.ItemsUpdatingScrollMode =
            _tailScrollActive || _isAtBottom
                ? ItemsUpdatingScrollMode.KeepLastItemInView
                : ItemsUpdatingScrollMode.KeepItemsInView;
    }

    private void QueueScrollStateUpdate() {
        if (_scrollStateUpdateScheduled)
            return;

        _scrollStateUpdateScheduled = true;
        if (!DispatcherQueue.TryEnqueue(DispatcherQueuePriority.Low, () => {
                _scrollStateUpdateScheduled = false;
                UpdateScrollState();
            }))
            _scrollStateUpdateScheduled = false;
    }

    public void ScrollToBottom() {
        if (!HasItems()) {
            _isAtBottom = true;
            UpdateVisualState();
            return;
        }

        RequestScrollToBottom();
    }

    private void OnJumpToLatestClick(object sender, RoutedEventArgs e) =>
        ScrollToBottom();

    private void OnUserScrollInput(object sender, PointerRoutedEventArgs e) {
        StopTailScroll();
    }

    private void UpdateVisualState() {
        var hasItems = HasItems();
        EmptyPresenter.Visibility = !hasItems && EmptyContent is not null
                                        ? Visibility.Visible
                                        : Visibility.Collapsed;
        JumpToLatestButton.Visibility = hasItems && !_isAtBottom &&
                                        !_tailScrollActive
                                            ? Visibility.Visible
                                            : Visibility.Collapsed;
    }

    private bool HasItems() {
        if (ItemsSource is null)
            return false;
        if (ItemsSource is ICollection collection)
            return collection.Count > 0;

        var enumerator = ItemsSource.GetEnumerator();
        try {
            return enumerator.MoveNext();
        } finally {
            (enumerator as IDisposable)?.Dispose();
        }
    }

    private bool TryGetLastItem(out object? lastItem) {
        lastItem = null;
        if (ItemsSource is null)
            return false;

        if (ItemsSource is IList list) {
            if (list.Count == 0)
                return false;
            lastItem = list[list.Count - 1];
            return true;
        }

        var found = false;
        foreach (var item in ItemsSource) {
            lastItem = item;
            found    = true;
        }
        return found;
    }

    private static T? FindDescendant<T>(DependencyObject root)
        where T : DependencyObject {
        var count = VisualTreeHelper.GetChildrenCount(root);
        for (var i = 0; i < count; i++) {
            var child = VisualTreeHelper.GetChild(root, i);
            if (child is T match)
                return match;
            if (FindDescendant<T>(child) is {} nested)
                return nested;
        }
        return null;
    }
}
