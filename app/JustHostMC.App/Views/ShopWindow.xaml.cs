using System;
using System.Linq;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;
using Windows.Graphics;

namespace JustHostMC.App.Views;

/// <summary>Navigation parameter shared by the shop pages.</summary>
public sealed record ShopNavArgs(ShopWindow Window, ShopViewModel Shop, ShopProjectItem? Project = null);

/// <summary>The mod shop: a separate window hosting home / search / detail
/// pages for browsing and installing mods from the configured sources.</summary>
public sealed partial class ShopWindow : Window
{
    private readonly Window? _owner;
    private readonly ILocalizer _localizer = new LocalizationService();
    private readonly DispatcherQueueTimer _suggestTimer;
    private string _pendingSuggestText = "";

    public ShopViewModel ViewModel { get; }

    public ShopWindow(ShopContext context)
    {
        ViewModel = new ShopViewModel(context, DispatcherQueue.GetForCurrentThread(), _localizer);
        InitializeComponent();

        var title = _localizer.Get("ShopWindow_Title");
        Title = title;
        ShopTitleBar.Title = title;
        ExtendsContentIntoTitleBar = true;
        AppWindow.Resize(new SizeInt32(1180, 760));

        _owner = App.Current.MainWindow;
        if (_owner is not null)
            _owner.Closed += OnOwnerClosed;
        Closed += OnClosed;

        _suggestTimer = DispatcherQueue.GetForCurrentThread().CreateTimer();
        _suggestTimer.Interval = TimeSpan.FromMilliseconds(300);
        _suggestTimer.IsRepeating = false;
        _suggestTimer.Tick += OnSuggestTimerTick;

        _ = InitializeAsync();
    }

    private async System.Threading.Tasks.Task InitializeAsync()
    {
        await ViewModel.LoadShopsAsync();
        foreach (var shop in ViewModel.Shops)
        {
            // SelectorBar items are built imperatively: an ItemsSource of plain
            // managed types plus an {x:Bind} template crashes at startup.
            var item = new SelectorBarItem { Text = shop.Name, Tag = shop, IsEnabled = shop.Ready };
            if (!shop.Ready)
                ToolTipService.SetToolTip(item, _localizer.Get("Shop_KeyMissingTooltip"));
            ShopSelector.Items.Add(item);
        }
        var selected = ShopSelector.Items.FirstOrDefault(i => ReferenceEquals(i.Tag, ViewModel.SelectedShop));
        if (selected is not null)
            ShopSelector.SelectedItem = selected;
        else
            NavigateHome();
    }

    private void OnShopSelectionChanged(SelectorBar sender, SelectorBarSelectionChangedEventArgs args)
    {
        if (sender.SelectedItem?.Tag is ShopInfo shop)
        {
            ViewModel.SelectedShop = shop;
            KeyMissingBar.IsOpen = !shop.Ready;
            NavigateHome();
        }
    }

    private void OnFilterChanged(object sender, RoutedEventArgs e) => NavigateCurrent();

    private void NavigateHome()
    {
        ContentFrame.BackStack.Clear();
        ContentFrame.Navigate(typeof(ShopHomePage), new ShopNavArgs(this, ViewModel));
    }

    /// <summary>Re-runs the current page after a filter change.</summary>
    private void NavigateCurrent()
    {
        if (ContentFrame.SourcePageType == typeof(ShopSearchPage))
            _ = ViewModel.StartSearchAsync();
        else if (ContentFrame.SourcePageType == typeof(ShopHomePage))
            _ = ViewModel.LoadHomeAsync();
    }

    private void OnBackClick(object sender, RoutedEventArgs e)
    {
        if (ContentFrame.CanGoBack)
            ContentFrame.GoBack();
    }

    private void OnFrameNavigated(object sender, NavigationEventArgs e) =>
        BackButton.IsEnabled = ContentFrame.CanGoBack;

    private void OnSearchTextChanged(AutoSuggestBox sender, AutoSuggestBoxTextChangedEventArgs args)
    {
        if (args.Reason != AutoSuggestionBoxTextChangeReason.UserInput)
            return;
        _pendingSuggestText = sender.Text;
        _suggestTimer.Stop();
        _suggestTimer.Start();
    }

    private async void OnSuggestTimerTick(DispatcherQueueTimer sender, object args)
    {
        var text = _pendingSuggestText;
        var suggestions = await ViewModel.GetSuggestionsAsync(text);
        // Ignore stale results typed past.
        if (text == _pendingSuggestText && text == SearchBox.Text)
            SearchBox.ItemsSource = suggestions;
    }

    private void OnSearchSubmitted(AutoSuggestBox sender, AutoSuggestBoxQuerySubmittedEventArgs args)
    {
        var query = (args.ChosenSuggestion as string) ?? args.QueryText;
        if (string.IsNullOrWhiteSpace(query))
            return;
        ViewModel.Query = query.Trim();
        if (ContentFrame.SourcePageType != typeof(ShopSearchPage))
            ContentFrame.Navigate(typeof(ShopSearchPage), new ShopNavArgs(this, ViewModel));
        _ = ViewModel.StartSearchAsync();
    }

    /// <summary>Navigates to a project's detail page (called by the pages).</summary>
    public void ShowProject(ShopProjectItem project) =>
        ContentFrame.Navigate(typeof(ShopDetailPage), new ShopNavArgs(this, ViewModel, project));

    private void OnOwnerClosed(object sender, WindowEventArgs args) => Close();

    private void OnClosed(object sender, WindowEventArgs args)
    {
        _suggestTimer.Stop();
        if (_owner is not null)
            _owner.Closed -= OnOwnerClosed;
    }
}
