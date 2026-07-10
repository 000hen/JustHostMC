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

        if (ViewModel.IsWebsiteAction) {
            await OpenWebsiteAsync();
            return;
        }

        if (ViewModel.IsModpack) {
            await CreateServerFlow(version);
            return;
        }

        var dependencies = ViewModel.MissingDependencies(version);
        IReadOnlyList<ShopDependency> chosen = [];
        if (dependencies.Count > 0) {
            var dialog = new ShopDependencySelectionDialog(dependencies) {
                XamlRoot = XamlRoot,
            };
            if (await dialog.ShowAsync() != ContentDialogResult.Primary)
                return;
            chosen = dialog.SelectedDependencies;
        }

        await ViewModel.InstallAsync(version, chosen);
    }

    // Prompts for a name + memory, then creates a new server from a modpack
    // version. Built imperatively to avoid ItemsControl/x:Bind template
    // pitfalls.
    private async Task CreateServerFlow(ShopVersionItem version) {
        var nameBox = new TextBox {
            Header = _localizer.Get("Shop_CreateServerNameLabel"),
            Text   = ViewModel.Card?.Title ?? "",
        };
        var memoryBox = new NumberBox {
            Header      = _localizer.Get("Shop_CreateServerMemoryLabel"),
            Value       = 4096,
            Minimum     = 512,
            Maximum     = 65536,
            SmallChange = 512,
            LargeChange = 1024,
            SpinButtonPlacementMode = NumberBoxSpinButtonPlacementMode.Inline,
        };
        var panel = new StackPanel { Spacing = 12 };
        panel.Children.Add(new TextBlock {
            Text = string.Format(_localizer.Get("Shop_CreateServerPromptBody"),
                                 version.Name),
            TextWrapping = TextWrapping.Wrap,
        });
        panel.Children.Add(nameBox);
        panel.Children.Add(memoryBox);

        var dialog = new ContentDialog {
            XamlRoot          = XamlRoot,
            Title             = _localizer.Get("Shop_CreateServerTitle"),
            Content           = panel,
            PrimaryButtonText = _localizer.Get("Shop_CreateServerConfirm"),
            CloseButtonText   = _localizer.Get("Common_Cancel"),
            DefaultButton     = ContentDialogButton.Primary,
        };
        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
            return;

        var name = nameBox.Text.Trim();
        if (name.Length == 0)
            name = ViewModel.Card?.Title ?? "Modpack";
        var memory =
            double.IsNaN(memoryBox.Value) ? 4096 : (int)memoryBox.Value;
        await ViewModel.CreateServerAsync(version, name, memory);
    }

    private async void OnOpenWebsite(object sender, RoutedEventArgs e) {
        await OpenWebsiteAsync();
    }

    private async Task OpenWebsiteAsync() {
        if (Uri.TryCreate(ViewModel.WebsiteUrl, UriKind.Absolute, out var uri))
            await Windows.System.Launcher.LaunchUriAsync(uri);
    }

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
