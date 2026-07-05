using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml.Data;
using Windows.Foundation;

namespace JustHostMC.App.ViewModels;

/// <summary>Window-scoped state for the mod shop: the available shop sources,
/// the relaxable compatibility filter, home sections, search results with
/// incremental paging, and search suggestions.</summary>
public sealed partial class ShopViewModel : ObservableObject
{
    private const int PageSize = 20;
    private const int SuggestionCount = 6;

    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;
    private int _homeGeneration;

    public ShopViewModel(ShopContext context, DispatcherQueue dispatcher, ILocalizer localizer)
    {
        Context = context;
        _dispatcher = dispatcher;
        _localizer = localizer;
        SearchResults = new ShopSearchResults(LoadSearchPageAsync);
    }

    public ShopContext Context { get; }

    public ObservableCollection<ShopInfo> Shops { get; } = new();

    public ObservableCollection<ShopSectionItem> HomeSections { get; } = new();

    public ShopSearchResults SearchResults { get; }

    public ObservableCollection<ShopCategoryFilter> CategoryFilters { get; } = new();

    [ObservableProperty]
    public partial ShopInfo? SelectedShop { get; set; }

    [ObservableProperty]
    public partial bool UseVersionFilter { get; set; } = true;

    [ObservableProperty]
    public partial bool UseLoaderFilter { get; set; } = true;

    [ObservableProperty]
    public partial bool IsHomeLoading { get; private set; }

    [ObservableProperty]
    public partial bool IsSearchLoading { get; private set; }

    [ObservableProperty]
    public partial string StatusMessage { get; private set; } = "";

    [ObservableProperty]
    public partial string Query { get; set; } = "";

    [ObservableProperty]
    public partial long TotalResults { get; private set; }

    public bool HasVersionFilter => Context.McVersion.Length > 0;
    public bool HasLoaderFilter => Context.Loader.Length > 0;
    public string VersionFilterLabel => Context.McVersion;
    public string LoaderFilterLabel => Context.Loader;
    public string SelectedShopName => SelectedShop?.Name ?? "";

    public bool HasCategoryFilters => CategoryFilters.Count > 0;

    partial void OnSelectedShopChanged(ShopInfo? value)
    {
        OnPropertyChanged(nameof(SelectedShopName));
        CategoryFilters.Clear();
        if (value?.Id == "modrinth")
        {
            foreach (var id in ModrinthCategories)
                CategoryFilters.Add(new ShopCategoryFilter(id, _localizer.Get($"shop.category.{id}")));
        }
        OnPropertyChanged(nameof(HasCategoryFilters));
    }

    private static readonly string[] ModrinthCategories =
    [
        "adventure", "decoration", "equipment", "game-mechanics", "library", "magic",
        "management", "mobs", "optimization", "storage", "technology", "transportation",
        "utility", "worldgen",
    ];

    public void ResetCategoryFilters()
    {
        foreach (var category in CategoryFilters)
            category.IsSelected = false;
    }

    private string EffectiveVersion => UseVersionFilter ? Context.McVersion : "";
    private string EffectiveLoader => UseLoaderFilter ? Context.Loader : "";

    /// <summary>Loads the shop list and selects the first ready source.</summary>
    public async Task LoadShopsAsync()
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            var list = await daemon.Shop.ListAsync(new Empty());
            await RunOnUIAsync(() =>
            {
                Shops.Clear();
                foreach (var shop in list.Shops)
                    Shops.Add(shop);
                SelectedShop = list.Shops.FirstOrDefault(s => s.Ready) ?? list.Shops.FirstOrDefault();
            });
        }
        catch
        {
            await RunOnUIAsync(() => StatusMessage = _localizer.Get("Shop_LoadFailed"));
        }
    }

    /// <summary>Reloads the landing-page sections for the selected shop.</summary>
    public async Task LoadHomeAsync()
    {
        var shop = SelectedShop;
        if (shop is null || !shop.Ready)
        {
            await RunOnUIAsync(HomeSections.Clear);
            return;
        }
        var generation = Interlocked.Increment(ref _homeGeneration);
        await RunOnUIAsync(() => { IsHomeLoading = true; StatusMessage = ""; });
        try
        {
            var daemon = await App.Current.DaemonReady;
            var reply = await daemon.Shop.HomeAsync(new ShopHomeRequest
            {
                ShopId = shop.Id,
                McVersion = EffectiveVersion,
                Loader = EffectiveLoader,
                Kind = Context.Kind,
            }, deadline: DateTime.UtcNow.AddSeconds(30));

            var sections = reply.Sections
                .Where(s => s.Projects.Count > 0)
                .Select(s => new ShopSectionItem(
                    _localizer.Get(s.Title.Key),
                    GetSectionDescription(s.Title.Key),
                    s.Projects.Select(p => new ShopProjectItem(p)).ToArray()))
                .ToArray();

            await RunOnUIAsync(() =>
            {
                if (generation != _homeGeneration)
                    return;
                HomeSections.Clear();
                foreach (var section in sections)
                    HomeSections.Add(section);
            });
        }
        catch
        {
            await RunOnUIAsync(() =>
            {
                if (generation == _homeGeneration)
                    StatusMessage = _localizer.Get("Shop_LoadFailed");
            });
        }
        finally
        {
            await RunOnUIAsync(() =>
            {
                if (generation == _homeGeneration)
                    IsHomeLoading = false;
            });
        }
    }

    private string GetSectionDescription(string titleKey)
    {
        var descriptionKey = $"{titleKey}.description";
        var description = _localizer.Get(descriptionKey);
        return description == descriptionKey ? "" : description;
    }

    /// <summary>Starts a fresh search for the current query/filters.</summary>
    public async Task StartSearchAsync()
    {
        await RunOnUIAsync(() =>
        {
            StatusMessage = "";
            TotalResults = 0;
            SearchResults.Reset();
        });
        // The ListView pulls the first page through ISupportIncrementalLoading,
        // but kick one load explicitly so an empty view can't stay empty.
        await SearchResults.LoadMoreItemsAsync(PageSize);
    }

    /// <summary>One page of search results; called by the incremental collection.</summary>
    private async Task<IReadOnlyList<ShopProjectItem>> LoadSearchPageAsync(int offset)
    {
        var shop = SelectedShop;
        if (shop is null || !shop.Ready)
            return Array.Empty<ShopProjectItem>();
        await RunOnUIAsync(() => IsSearchLoading = true);
        try
        {
            var daemon = await App.Current.DaemonReady;
            var request = new ShopSearchRequest
            {
                ShopId = shop.Id,
                Query = Query,
                McVersion = EffectiveVersion,
                Loader = EffectiveLoader,
                Kind = Context.Kind,
                Sort = Sort,
                Offset = offset,
                Limit = PageSize,
            };
            request.Categories.AddRange(CategoryFilters.Where(c => c.IsSelected).Select(c => c.Id));
            var page = await daemon.Shop.SearchAsync(request, deadline: DateTime.UtcNow.AddSeconds(30));
            await RunOnUIAsync(() => TotalResults = page.Total);
            return page.Projects.Select(p => new ShopProjectItem(p)).ToArray();
        }
        catch
        {
            await RunOnUIAsync(() => StatusMessage = _localizer.Get("Shop_LoadFailed"));
            return Array.Empty<ShopProjectItem>();
        }
        finally
        {
            await RunOnUIAsync(() => IsSearchLoading = false);
        }
    }

    [ObservableProperty]
    public partial ShopSort Sort { get; set; } = ShopSort.Relevance;

    /// <summary>Fetches up to six title suggestions for the suggestion dropdown.</summary>
    public async Task<IReadOnlyList<string>> GetSuggestionsAsync(string text)
    {
        var shop = SelectedShop;
        if (shop is null || !shop.Ready || text.Trim().Length < 2)
            return Array.Empty<string>();
        try
        {
            var daemon = await App.Current.DaemonReady;
            var request = new ShopSearchRequest
            {
                ShopId = shop.Id,
                Query = text,
                McVersion = EffectiveVersion,
                Loader = EffectiveLoader,
                Kind = Context.Kind,
                Sort = ShopSort.Relevance,
                Offset = 0,
                Limit = SuggestionCount,
            };
            request.Categories.AddRange(CategoryFilters.Where(c => c.IsSelected).Select(c => c.Id));
            var page = await daemon.Shop.SearchAsync(request, deadline: DateTime.UtcNow.AddSeconds(10));
            return page.Projects.Select(p => p.Title).ToArray();
        }
        catch
        {
            return Array.Empty<string>();
        }
    }

    private Task RunOnUIAsync(Action action)
    {
        if (_dispatcher.HasThreadAccess)
        {
            action();
            return Task.CompletedTask;
        }
        var completion = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        if (!_dispatcher.TryEnqueue(() =>
            {
                try
                {
                    action();
                    completion.SetResult();
                }
                catch (Exception ex)
                {
                    completion.SetException(ex);
                }
            }))
        {
            completion.SetException(new InvalidOperationException("The UI dispatcher is unavailable."));
        }
        return completion.Task;
    }
}

public sealed partial class ShopCategoryFilter : ObservableObject
{
    public ShopCategoryFilter(string id, string label)
    {
        Id = id;
        Label = label;
    }

    public string Id { get; }
    public string Label { get; }

    [ObservableProperty]
    public partial bool IsSelected { get; set; }
}

/// <summary>Search results with offset paging: the ListView keeps calling
/// LoadMoreItemsAsync while the user scrolls and more pages exist.</summary>
public sealed class ShopSearchResults : ObservableCollection<ShopProjectItem>, ISupportIncrementalLoading
{
    private readonly Func<int, Task<IReadOnlyList<ShopProjectItem>>> _loadPage;
    private bool _exhausted;
    private int _generation;
    private int _loadingGeneration = -1;

    public ShopSearchResults(Func<int, Task<IReadOnlyList<ShopProjectItem>>> loadPage)
    {
        _loadPage = loadPage;
    }

    public bool HasMoreItems => !_exhausted;

    public void Reset()
    {
        _generation++;
        Clear();
        _exhausted = false;
    }

    public IAsyncOperation<LoadMoreItemsResult> LoadMoreItemsAsync(uint count)
    {
        return System.Runtime.InteropServices.WindowsRuntime.AsyncInfo.Run(async _ =>
        {
            var generation = _generation;
            if (_loadingGeneration == generation || _exhausted)
                return new LoadMoreItemsResult { Count = 0 };
            _loadingGeneration = generation;
            try
            {
                var page = await _loadPage(Count);
                if (generation != _generation)
                    return new LoadMoreItemsResult { Count = 0 };
                if (page.Count == 0)
                    _exhausted = true;
                foreach (var item in page)
                    Add(item);
                return new LoadMoreItemsResult { Count = (uint)page.Count };
            }
            finally
            {
                if (_loadingGeneration == generation)
                    _loadingGeneration = -1;
            }
        });
    }
}
