using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace JustHostMC.App.Views;

/// <summary>Search results with incremental paging and sort selection.</summary>
public sealed partial class ShopSearchPage : Page
{
    private static readonly ShopSort[] SortOrder =
    [
        ShopSort.Relevance, ShopSort.Downloads, ShopSort.Follows, ShopSort.Newest, ShopSort.Updated,
    ];

    private readonly ILocalizer _localizer = new LocalizationService();
    private ShopWindow? _window;
    private bool _ready;

    public ShopViewModel ViewModel { get; private set; } = null!;

    public ShopSearchPage()
    {
        InitializeComponent();
    }

    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        base.OnNavigatedTo(e);
        var args = (ShopNavArgs)e.Parameter;
        _window = args.Window;
        ViewModel = args.Shop;
        var index = System.Array.IndexOf(SortOrder, ViewModel.Sort);
        SortCombo.SelectedIndex = index >= 0 ? index : 0;
        _ready = true;
        Bindings.Update();
    }

    private void OnSortChanged(object sender, SelectionChangedEventArgs e)
    {
        if (!_ready || SortCombo.SelectedIndex < 0)
            return;
        ViewModel.Sort = SortOrder[SortCombo.SelectedIndex];
        _ = ViewModel.StartSearchAsync();
    }

    private void OnItemClick(object sender, ItemClickEventArgs e)
    {
        if (e.ClickedItem is ShopProjectItem item)
            _window?.ShowProject(item);
    }

    public string ResultSummary(long total, string query) =>
        string.Format(_localizer.Get("Shop_ResultSummary"), total, query);

    public Visibility ShowEmpty(bool loading, long total) =>
        !loading && total == 0 && ViewModel.SearchResults.Count == 0
            ? Visibility.Visible
            : Visibility.Collapsed;
}
