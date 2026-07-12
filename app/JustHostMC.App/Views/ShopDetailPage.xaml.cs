using System.ComponentModel;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace JustHostMC.App.Views;

/// <summary>Project detail: hero, gallery, rendered description, versions,
/// and the install flow with a required-dependency confirmation.</summary>
public sealed partial class ShopDetailPage : Page {
    private readonly ILocalizer _localizer = new LocalizationService();
    private bool _webViewReady;

    public ShopDetailViewModel ViewModel { get; private set; } = null!;

    public ShopDetailPage() {
        InitializeComponent();
    }

    protected override async void OnNavigatedTo(NavigationEventArgs e) {
        base.OnNavigatedTo(e);
        var args  = (ShopNavArgs)e.Parameter;
        ViewModel = new ShopDetailViewModel(args.Shop, args.Project!,
                                            DispatcherQueue, _localizer);
        Bindings.Update();
        ViewModel.PropertyChanged += OnViewModelPropertyChanged;

        var dark = ActualTheme == ElementTheme.Dark;
        await ViewModel.LoadAsync(dark);
    }

    protected override void OnNavigatedFrom(NavigationEventArgs e) {
        base.OnNavigatedFrom(e);
        if (ViewModel is not null)
            ViewModel.PropertyChanged -= OnViewModelPropertyChanged;
    }

    private async void OnViewModelPropertyChanged(object? sender,
                                                  PropertyChangedEventArgs e) {
        if (e.PropertyName != nameof(ShopDetailViewModel.BodyHtml) ||
            ViewModel.BodyHtml.Length == 0)
            return;
        try {
            // WebView2 initializes asynchronously; NavigateToString needs the
            // core.
            if (!_webViewReady) {
                await BodyView.EnsureCoreWebView2Async();
                BodyView.CoreWebView2.Settings.AreDefaultContextMenusEnabled =
                    false;
                BodyView.CoreWebView2.Settings.IsZoomControlEnabled = false;
                // External links open in the default browser, never in-place.
                BodyView.CoreWebView2.NewWindowRequested +=
                    (sender2, args2) => {
                        args2.Handled = true;
                        _             = Windows.System.Launcher.LaunchUriAsync(
                            new Uri(args2.Uri));
                    };
                BodyView.CoreWebView2.NavigationStarting +=
                    (sender2, args2) => {
                        if (args2.Uri.StartsWith(
                                "http", StringComparison.OrdinalIgnoreCase)) {
                            args2.Cancel = true;
                            _ = Windows.System.Launcher.LaunchUriAsync(
                                new Uri(args2.Uri));
                        }
                    };
                _webViewReady = true;
            }
            BodyView.NavigateToString(ViewModel.BodyHtml);
        } catch {
            // A missing WebView2 runtime leaves the overview blank; the rest of
            // the page (versions, install) still works.
        }
    }

    private void OnTabChanged(SelectorBar sender,
                              SelectorBarSelectionChangedEventArgs args) {
        var versions = ReferenceEquals(sender.SelectedItem, VersionsTab);
        OverviewPanel.Visibility =
            versions ? Visibility.Collapsed : Visibility.Visible;
        VersionsPanel.Visibility =
            versions ? Visibility.Visible : Visibility.Collapsed;
    }

    private async void OnInstallClick(object sender, RoutedEventArgs e) {
        if (sender is not FrameworkElement { Tag : ShopVersionItem version })
            return;

        var dependencies = ViewModel.MissingDependencies(version);
        var chosen       = new List<ShopDependency>();
        if (dependencies.Count > 0) {
            var picks =
                dependencies
                    .Select(d => new CheckBox {
                        Content   = d.Title.Length > 0 ? d.Title : d.ProjectId,
                        IsChecked = true,
                        Tag       = d,
                    })
                    .ToArray();
            var dialog = (ContentDialog)Resources["DependencyPromptDialog"];
            var scroller = (ScrollViewer)dialog.Content;
            var panel = (StackPanel)scroller.Content;
            foreach (var pick in picks) panel.Children.Add(pick);

            dialog.XamlRoot = XamlRoot;
            try {
                if (await dialog.ShowAsync() != ContentDialogResult.Primary)
                    return;
                chosen.AddRange(picks.Where(p => p.IsChecked == true)
                                    .Select(p => (ShopDependency)p.Tag));
            } finally {
                foreach (var pick in picks) panel.Children.Remove(pick);
            }
        }

        await ViewModel.InstallAsync(version, chosen);
    }

    private async void OnOpenWebsite(object sender, RoutedEventArgs e) {
        if (Uri.TryCreate(ViewModel.WebsiteUrl, UriKind.Absolute, out var uri))
            await Windows.System.Launcher.LaunchUriAsync(uri);
    }

    public Visibility BoolToVisibility(bool value) =>
        value ? Visibility.Visible : Visibility.Collapsed;

    public Visibility BodyVisibility(bool loading, string body) =>
        !loading && body.Length > 0? Visibility.Visible : Visibility.Collapsed;

    public Visibility HasUrl(string url) => url.Length >
                                            0? Visibility.Visible
        : Visibility.Collapsed;

    public bool HasStatus(string status) => status.Length > 0;

    public double ProgressPercent(double fraction) => fraction * 100;

    public InfoBarSeverity StatusSeverity(bool succeeded) =>
        succeeded ? InfoBarSeverity.Success : InfoBarSeverity.Error;

    public Visibility ShowNoVersions(bool loading,
                                     int count) => !loading && count == 0
                                                       ? Visibility.Visible
                                                       : Visibility.Collapsed;
}
