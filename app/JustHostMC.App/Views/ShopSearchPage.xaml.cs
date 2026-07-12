using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace JustHostMC.App.Views;

/// <summary>Search results with incremental paging and sort
/// selection.</summary>
public sealed partial class ShopSearchPage : Page {
    private static readonly ShopSort[] SortOrder = [
        ShopSort.Relevance,
        ShopSort.Downloads,
        ShopSort.Follows,
        ShopSort.Newest,
        ShopSort.Updated,
    ];

    private readonly ILocalizer _localizer = new LocalizationService();
    private ShopWindow? _window;
    private bool _ready;
    private bool _updatingSort;

    public ShopViewModel ViewModel { get; private set; } = null!;

    public IReadOnlyList<int> SkeletonItems { get; } = [0, 1, 2, 3, 4, 5];

    public ShopSearchPage() {
        // The result collection is window-scoped, so keep this page's visual
        // tree with it. Recreating the ListView after visiting a project detail
        // resets its internal ScrollViewer to the top.
        NavigationCacheMode = NavigationCacheMode.Required;
        InitializeComponent();
    }

    protected override void OnNavigatedTo(NavigationEventArgs e) {
        base.OnNavigatedTo(e);
        var args      = (ShopNavArgs)e.Parameter;
        _window       = args.Window;
        ViewModel     = args.Shop;
        var index     = System.Array.IndexOf(SortOrder, ViewModel.Sort);
        _updatingSort = true;
        WideSortCombo.SelectedIndex    = index >= 0 ? index : 0;
        CompactSortCombo.SelectedIndex = index >= 0 ? index : 0;
        _updatingSort                  = false;
        _ready                         = true;
        Bindings.Update();
    }

    private void OnWideSortChanged(object sender,
                                   SelectionChangedEventArgs e) =>
        ApplySort(WideSortCombo.SelectedIndex, CompactSortCombo);

    private void OnCompactSortChanged(object sender,
                                      SelectionChangedEventArgs e) =>
        ApplySort(CompactSortCombo.SelectedIndex, WideSortCombo);

    private void ApplySort(int selectedIndex, ComboBox peer) {
        if (!_ready || _updatingSort || selectedIndex < 0)
            return;
        _updatingSort      = true;
        peer.SelectedIndex = selectedIndex;
        _updatingSort      = false;
        ViewModel.Sort     = SortOrder[selectedIndex];
        _                  = ViewModel.StartSearchAsync();
    }

    private void OnVersionFilterChanged(object sender, RoutedEventArgs e) {
        if (!_ready || sender is not CheckBox checkBox)
            return;
        ViewModel.UseVersionFilter = checkBox.IsChecked == true;
        _                          = ViewModel.StartSearchAsync();
    }

    private void OnLoaderFilterChanged(object sender, RoutedEventArgs e) {
        if (!_ready || sender is not CheckBox checkBox)
            return;
        ViewModel.UseLoaderFilter = checkBox.IsChecked == true;
        _                         = ViewModel.StartSearchAsync();
    }

    private void OnCategoryFilterChanged(object sender, RoutedEventArgs e) {
        if (!_ready || sender is not
            CheckBox { Tag : ShopCategoryFilter category } checkBox)
            return;
        category.IsSelected = checkBox.IsChecked == true;
        _                   = ViewModel.StartSearchAsync();
    }

    private void OnResetFilters(object sender, RoutedEventArgs e) {
        if (!_ready)
            return;
        ViewModel.UseVersionFilter = ViewModel.HasVersionFilter;
        ViewModel.UseLoaderFilter  = ViewModel.HasLoaderFilter;
        ViewModel.ResetCategoryFilters();
        ViewModel.Sort                 = ShopSort.Relevance;
        _updatingSort                  = true;
        WideSortCombo.SelectedIndex    = 0;
        CompactSortCombo.SelectedIndex = 0;
        _updatingSort                  = false;
        _                              = ViewModel.StartSearchAsync();
    }

    private void OnItemClick(object sender, ItemClickEventArgs e) {
        if (e.ClickedItem is ShopProjectItem item)
            _window?.ShowProject(item);
    }

    public string ResultSummary(long total, string query) =>
        string.Format(_localizer.Get("Shop.ResultSummary"), total, query);

    public Visibility ShowEmpty(bool loading, long total) =>
        !loading && total == 0 && ViewModel.SearchResults.Count == 0
            ? Visibility.Visible
            : Visibility.Collapsed;

    public Visibility ShowSkeleton(bool loading, int resultCount) =>
        loading && resultCount == 0 ? Visibility.Visible : Visibility.Collapsed;

    public Visibility ShowLoadMoreIndicator(bool loading, int resultCount) =>
        loading && resultCount > 0? Visibility.Visible : Visibility.Collapsed;

    public Visibility BoolToVisibility(bool value) =>
        value ? Visibility.Visible : Visibility.Collapsed;
}
