
using System.ComponentModel;
using System.Diagnostics;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;
using Windows.Storage.Pickers;

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
        var dialog = new BackupsContentDialog(
            Server.Id, Server.Name, Server.IsRunning, DispatcherQueue) {
            XamlRoot = XamlRoot,
        };
        await dialog.ShowAsync();
    }

    // ── Modpack update / export
    // ────────────────────────────────────────────

    /// <summary>Splits "packId/versionId"; returns false for non-modpack
    /// servers.</summary>
    private bool TryGetPackIdentity(out string packId, out string versionId) {
        var parts = Server.ProviderVersion.Split('/', 2);
        if (parts.Length == 2 && parts[0].Length > 0 && parts[1].Length > 0) {
            packId    = parts[0];
            versionId = parts[1];
            return true;
        }
        packId    = "";
        versionId = "";
        return false;
    }

    private async void OnUpdateModpackClick(object sender, RoutedEventArgs e) {
        // Imported packs have no upstream source to update from (the menu item
        // is hidden for them; this guards a programmatic call).
        if (Server.ProviderId == "import")
            return;
        if (!TryGetPackIdentity(out var packId, out var currentVersionId))
            return;

        ShopVersionList list;
        try {
            var daemon = await App.Current.DaemonReady;
            list       = await daemon.Shop.GetVersionsAsync(
                new ShopVersionsRequest {
                    ShopId    = Server.ProviderId,
                    ProjectId = packId,
                },
                deadline: DateTime.UtcNow.AddSeconds(30));
        } catch (Exception) {
            list = new ShopVersionList();
        }
        var current =
            list.Versions.FirstOrDefault(v => v.Id == currentVersionId);
        var choices =
            list.Versions.Where(v => v.Id != currentVersionId).ToArray();
        var currentName =
            current is null ? currentVersionId
                            : (current.Name.Length > 0 ? current.Name
                                                       : current.VersionNumber);
        var dialog =
            new ServerUpdateModpackDialog(Server.Name, currentName, choices) {
                XamlRoot = XamlRoot,
            };
        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
            return;
        if (dialog.SelectedVersion is not {} picked)
            return;
        _ = _main.UpdateModpackAsync(Server, $"{packId}/{picked.Id}");
    }

    private async void OnExportModpackClick(object sender, RoutedEventArgs e) {
        if (!TryGetPackIdentity(out _, out _))
            return;
        var picker = new FileSavePicker {
            SuggestedFileName      = Server.Name,
            SuggestedStartLocation = PickerLocationId.DocumentsLibrary,
        };
        picker.FileTypeChoices.Add(".zip", new List<string> { ".zip" });
        var hwnd =
            WinRT.Interop.WindowNative.GetWindowHandle(App.Current.MainWindow);
        WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);

        var file = await picker.PickSaveFileAsync();
        if (file is not null)
            _ = _main.ExportModpackAsync(Server, file.Path);
    }

    private async void OnTitleRenameClick(object sender, RoutedEventArgs e) =>
        await ShowRenameDialogAsync();

    private async void OnEditClick(object sender, RoutedEventArgs e) =>
        await ShowEditDialogAsync();

    private async Task ShowEditDialogAsync() {
        var dialog = new EditServerDialog(_main, Server) {
            XamlRoot = XamlRoot,
        };

        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            await _main.UpdateServerAsync(dialog.BuildUpdateRequest());
    }

    private async Task ShowRenameDialogAsync() {
        var dialog = new RenameServerDialog(Server.Name) {
            XamlRoot = XamlRoot,
        };

        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            await _main.RenameServerAsync(Server, dialog.ServerName);
    }

    private async void OnOpenInstanceFolderClick(object sender,
                                                 RoutedEventArgs e) {
        var folder = ResolveInstanceFolder();
        if (folder is null) {
            var dialog = new ServerFolderNotFoundDialog {
                XamlRoot = XamlRoot,
            };
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
        ContentDialog confirm = Server.IsIncompleteInstallation
                                    ? new IncompleteServerRemovalDialog()
                                    : new DeleteServerDialog();
        confirm.XamlRoot      = XamlRoot;
        if (await confirm.ShowAsync() != ContentDialogResult.Primary)
            return;
        _main.DeleteServerCommand.Execute(Server);
        _shell.RequestHome();
    }
}
