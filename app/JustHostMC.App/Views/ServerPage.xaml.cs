
using System.ComponentModel;
using System.Diagnostics;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace JustHostMC.App.Views;

/// <summary>One server's page: a collapsing metadata header with a
/// green/red/gray state button, and a SelectorBar switching the Console,
/// Players, Performance, and Plugins/Mods detail panels.</summary>
public sealed partial class ServerPage : Page {
    private readonly ILocalizer _localizer = new LocalizationService();
    private NavShellViewModel _shell       = null!;
    private MainViewModel _main            = null!;

    public ServerPage() => InitializeComponent();

    public ServerItem Server { get; private set; }            = null!;
    public ConsoleViewModel Console { get; private set; }     = null!;
    public PlayersViewModel Players { get; private set; }     = null!;
    public MetricsViewModel Metrics { get; private set; }     = null!;
    public ModsViewModel Mods { get; private set; }           = null!;
    public ServerConfigViewModel Config { get; private set; } = null!;

    protected override void OnNavigatedTo(NavigationEventArgs e) {
        var args = (ServerPageArgs)e.Parameter;
        Server   = args.Server;
        _shell   = args.Shell;
        _main    = args.Shell.Main;

        // Reuse cached VMs (keeps gRPC streams alive across page visits).
        var cache = _shell.GetOrCreateServerCache(Server.Id, Server.Name,
                                                  DispatcherQueue, _localizer);
        Console   = cache.Console;
        Players   = cache.Players;
        Metrics   = cache.Metrics;
        Mods      = cache.Mods;
        Config    = cache.Config;

        Server.PropertyChanged += OnServerPropertyChanged;
        Mods.SetServerStopped(IsStopped(Server.Status));
        Config.SetServerStopped(IsStopped(Server.Status));

        // Hide the Mods/Plugins section for providers whose mod_layout is
        // "none". The catalog may load after navigation, so refresh when it
        // becomes ready.
        _main.ProviderCatalog.Loaded += OnProviderCatalogLoaded;
        ApplyModsCapability();
        // Eagerly load the catalog so capability resolves even if this is the
        // first page.
        _ = RunSilentlyAsync(_main.GetProvidersAsync());

        // Attach live streams immediately, then warm heavier tab data after the
        // page has had a chance to render.
        _ = RunSilentlyAsync(cache.AttachAsync());
        DispatcherQueue.TryEnqueue(async () => {
            await Task.Delay(150);
            await RunSilentlyAsync(cache.PreloadAsync());
        });
    }

    protected override void OnNavigatedFrom(NavigationEventArgs e) {
        // Unsubscribe UI handlers; VMs stay alive in the cache.
        Server.PropertyChanged -= OnServerPropertyChanged;
        _main.ProviderCatalog.Loaded -= OnProviderCatalogLoaded;
    }

    private void OnProviderCatalogLoaded() =>
        DispatcherQueue.TryEnqueue(ApplyModsCapability);

    /// <summary>Shows/hides the Mods section based on the provider's mod_layout
    /// capability.</summary>
    private void ApplyModsCapability() {
        // Default to supported until the catalog resolves this provider (null
        // layout).
        var supportsMods =
            _main.ProviderCatalog.ModLayoutFor(Server.ProviderId) != "none";
        ModsSectionItem.Visibility = Show(supportsMods);

        // If the active section just got hidden, fall back to the console.
        if (!supportsMods && SectionBar.SelectedItem == ModsSectionItem) {
            SectionBar.SelectedItem =
                SectionBar.Items.Count > 0 ? SectionBar.Items[0] : null;
        }
    }

    private void OnServerPropertyChanged(object? sender,
                                         PropertyChangedEventArgs e) {
        if (e.PropertyName == nameof(ServerItem.Status)) {
            Mods.SetServerStopped(IsStopped(Server.Status));
            Config.SetServerStopped(IsStopped(Server.Status));
        }
    }

    // ── Header state button
    // ───────────────────────────────────────────────────

    private void OnStateButtonClick(object sender, RoutedEventArgs e) {
        if (Server.CanStart)
            _main.StartServerCommand.Execute(Server);
        else if (Server.CanStop)
            _main.StopServerCommand.Execute(Server);
    }

    private static bool IsStopped(ServerStatus s) =>
        s is ServerStatus.Stopped or ServerStatus.Crashed;

    // ── Sections, scroll, and commands
    // ─────────────────────────────────────────

    private void OnSectionChanged(SelectorBar sender,
                                  SelectorBarSelectionChangedEventArgs args) {
        var tag = sender.SelectedItem?.Tag as string ?? "console";
        ConsolePanel.Visibility     = Show(tag == "console");
        PlayersPanel.Visibility     = Show(tag == "players");
        ConfigPanel.Visibility      = Show(tag == "config");
        PerformancePanel.Visibility = Show(tag == "performance");
        ModsPanel.Visibility        = Show(tag == "mods");

        if (tag == "config" && Config is not null) {
            Config.PrepareInitialLoad();
            _ = RunSilentlyAsync(Config.EnsureLoadedAsync());
        } else if (tag == "mods" && Mods is not null)
            _ = RunSilentlyAsync(Mods.EnsureLoadedAsync());
    }

    private static Visibility Show(bool visible) => visible
                                                        ? Visibility.Visible
                                                        : Visibility.Collapsed;

    private static async Task RunSilentlyAsync(Task task) {
        try {
            await task;
        } catch {
            // View models surface transient load failures through their own
            // status messages; background warm-up should not make navigation
            // noisy.
        }
    }

    private async void OnBackupsClick(object sender, RoutedEventArgs e) {
        var content = new BackupsDialog(Server.Id, Server.Name,
                                        Server.IsRunning, DispatcherQueue);
        var dialog = (ContentDialog)Resources["BackupsHostDialog"];
        dialog.XamlRoot = XamlRoot;
        dialog.Title    = Server.Name;
        dialog.Content  = content;
        async void OnOpened(ContentDialog sender,
                            ContentDialogOpenedEventArgs args) =>
            await content.LoadAsync();
        dialog.Opened += OnOpened;
        ContentDialogSizing.Apply(dialog, useWideLayout: true);
        try {
            await dialog.ShowAsync();
        } finally {
            dialog.Opened -= OnOpened;
            dialog.Content = null;
            dialog.Title = null;
        }
    }

    private async void OnTitleRenameClick(object sender, RoutedEventArgs e) =>
        await ShowRenameDialogAsync();

    private async void OnEditClick(object sender, RoutedEventArgs e) =>
        await ShowEditDialogAsync();

    private async Task ShowEditDialogAsync() {
        var content = new ServerDialog(_main, ServerDialogMode.Edit, Server);
        var dialog = (ContentDialog)Resources["EditServerDialog"];
        dialog.XamlRoot               = XamlRoot;
        dialog.Content                = content;
        dialog.IsPrimaryButtonEnabled = content.CanSubmit;
        void OnCanSubmitChanged(object? sender, EventArgs args) =>
            dialog.IsPrimaryButtonEnabled = content.CanSubmit;
        content.CanSubmitChanged += OnCanSubmitChanged;
        ContentDialogSizing.Apply(dialog);

        ContentDialogResult result;
        try {
            result = await dialog.ShowAsync();
        } finally {
            content.CanSubmitChanged -= OnCanSubmitChanged;
            dialog.Content = null;
            dialog.IsPrimaryButtonEnabled = false;
        }

        if (result == ContentDialogResult.Primary)
            await _main.UpdateServerAsync(content.BuildUpdateRequest());
    }

    private async Task ShowRenameDialogAsync() {
        var dialog  = (ContentDialog)Resources["RenameServerDialog"];
        var nameBox = (TextBox)dialog.Content;
        dialog.XamlRoot        = XamlRoot;
        nameBox.Text            = Server.Name;
        nameBox.SelectionStart  = 0;
        nameBox.SelectionLength = Server.Name.Length;
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            await _main.RenameServerAsync(Server, nameBox.Text);
    }

    private async void OnOpenInstanceFolderClick(object sender,
                                                 RoutedEventArgs e) {
        var folder = ResolveInstanceFolder();
        if (folder is null) {
            var dialog =
                (ContentDialog)Resources["ServerFolderMissingDialog"];
            dialog.XamlRoot = XamlRoot;
            await dialog.ShowAsync();
            return;
        }

        Process.Start(new ProcessStartInfo {
            FileName        = folder,
            UseShellExecute = true,
        });
    }

    private string? ResolveInstanceFolder() {
        var roots =
            new[] {
                GetPackagedDataRoot(),
                Path.Combine(
                    Environment.GetFolderPath(
                        Environment.SpecialFolder.LocalApplicationData),
                    "JustHostMC"),
            }
                .Where(p => !string.IsNullOrWhiteSpace(p))
                .Distinct(StringComparer.OrdinalIgnoreCase);

        foreach (var root in roots) {
            foreach (var candidate in InstanceFolderCandidates(root!)) {
                if (Directory.Exists(candidate))
                    return candidate;
            }
        }

        return null;
    }

    private IEnumerable<string> InstanceFolderCandidates(string root) {
        yield return Path.Combine(root, "servers", Server.Id);
        yield return Path.Combine(root, "instances", Server.Id);
        yield return Path.Combine(root, Server.Id);
        yield return Path.Combine(root, "servers", Server.Name);
        yield return Path.Combine(root, "instances", Server.Name);
    }

    private static string? GetPackagedDataRoot() {
        try {
            return Windows.Storage.ApplicationData.Current.LocalFolder.Path;
        } catch {
            return null;
        }
    }

    private async void OnDeleteClick(object sender, RoutedEventArgs e) {
        var confirm = (ContentDialog)Resources["DeleteServerDialog"];
        confirm.XamlRoot = XamlRoot;
        if (await confirm.ShowAsync() != ContentDialogResult.Primary)
            return;
        _main.DeleteServerCommand.Execute(Server);
        _shell.RequestHome();
    }
}
