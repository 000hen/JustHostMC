using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Navigation;
using System;
using System.Collections.Specialized;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using Windows.Storage.Pickers;
using Windows.System;

namespace JustHostMC.App.Views;

/// <summary>One server's page: a collapsing metadata header with a green/red/gray
/// state button, and a SelectorBar switching the Console, Players, Performance, and
/// Plugins/Mods detail panels.</summary>
public sealed partial class ServerPage : Page {
    private readonly ILocalizer _localizer = new LocalizationService();
    private NavShellViewModel _shell = null!;
    private MainViewModel _main = null!;

    public ServerPage() => InitializeComponent();

    public ServerItem Server { get; private set; } = null!;
    public ConsoleViewModel Console { get; private set; } = null!;
    public PlayersViewModel Players { get; private set; } = null!;
    public MetricsViewModel Metrics { get; private set; } = null!;
    public ModsViewModel Mods { get; private set; } = null!;

    protected override async void OnNavigatedTo(NavigationEventArgs e) {
        var args = (ServerPageArgs)e.Parameter;
        Server = args.Server;
        _shell = args.Shell;
        _main = args.Shell.Main;

        // Reuse cached VMs (keeps gRPC streams alive across page visits).
        var cache = _shell.GetOrCreateServerCache(
            Server.Id, Server.Name, DispatcherQueue, _localizer);
        Console = cache.Console;
        Players = cache.Players;
        Metrics = cache.Metrics;
        Mods = cache.Mods;

        Server.PropertyChanged += OnServerPropertyChanged;
        Console.Lines.CollectionChanged += OnConsoleLinesChanged;
        Mods.SetServerStopped(IsStopped(Server.Status));

        // AttachAsync is a no-op on revisits; first call runs all 4 in parallel.
        await cache.AttachAsync();
    }

    protected override void OnNavigatedFrom(NavigationEventArgs e) {
        // Unsubscribe UI handlers; VMs stay alive in the cache.
        Server.PropertyChanged -= OnServerPropertyChanged;
        Console.Lines.CollectionChanged -= OnConsoleLinesChanged;
    }

    private void OnServerPropertyChanged(object? sender, PropertyChangedEventArgs e) {
        if (e.PropertyName == nameof(ServerItem.Status))
            Mods.SetServerStopped(IsStopped(Server.Status));
    }

    private void OnConsoleLinesChanged(object? sender, NotifyCollectionChangedEventArgs e)
        => DispatcherQueue.TryEnqueue(() => {
            if (Console.Lines.Count > 0)
                ConsoleLogScroller.ScrollIntoView(Console.Lines[^1]);
        });

    // ── Header state button ───────────────────────────────────────────────────

    private void OnPageSizeChanged(object sender, SizeChangedEventArgs e) => UpdateResponsiveLayout(e.NewSize.Width);

    private void UpdateResponsiveLayout(double width) {
        var wide = width >= 900;
        var medium = !wide && width >= 620;

        MetaWide.Visibility = Show(wide);
        MetaMedium.Visibility = Show(medium);
        MetaNarrow.Visibility = Show(!wide && !medium);
    }

    private void OnStateButtonClick(object sender, RoutedEventArgs e) {
        if (Server.CanStart)
            _main.StartServerCommand.Execute(Server);
        else if (Server.CanStop)
            _main.StopServerCommand.Execute(Server);
    }

    private Brush StateBrush(ServerStatus s) => (Brush)Application.Current.Resources[s switch {
        ServerStatus.Running => "SystemFillColorCriticalBrush",
        ServerStatus.Starting or ServerStatus.Stopping or ServerStatus.Installing => "ControlFillColorDisabledBrush",
        _ => "SystemFillColorSuccessBrush",
    }];

    private Brush StateForeground(ServerStatus s) => (Brush)Application.Current.Resources[
        s is ServerStatus.Starting or ServerStatus.Stopping or ServerStatus.Installing
            ? "TextFillColorDisabledBrush"
            : "TextOnAccentFillColorPrimaryBrush"];

    private bool StateEnabled(ServerStatus s) => s is ServerStatus.Stopped or ServerStatus.Crashed or ServerStatus.Running;

    private string PortText(int port) => _localizer.Get("Server_PortLabel", ("port", port.ToString()));

    private string PortValueText(int port) => port > 0
        ? port.ToString()
        : _localizer.Get("Server_PortAutoValue");

    private string MemoryText(int memoryMb) => memoryMb > 0
        ? _localizer.Get("Server_MemoryValue", ("memory", memoryMb.ToString()))
        : _localizer.Get("Server_ValueUnknown");

    private string PlayersHeader(int count) => _localizer.Get("Players_Header", ("count", count.ToString()));

    private Visibility HasNoPlayers(int count) => count == 0 ? Visibility.Visible : Visibility.Collapsed;

    private static bool IsStopped(ServerStatus s) => s is ServerStatus.Stopped or ServerStatus.Crashed;

    // ── Sections, scroll, and commands ─────────────────────────────────────────

    private void OnSectionChanged(SelectorBar sender, SelectorBarSelectionChangedEventArgs args) {
        var tag = sender.SelectedItem?.Tag as string ?? "console";
        ConsolePanel.Visibility = Show(tag == "console");
        PlayersPanel.Visibility = Show(tag == "players");
        PerformancePanel.Visibility = Show(tag == "performance");
        ModsPanel.Visibility = Show(tag == "mods");
    }

    private static Visibility Show(bool visible) => visible ? Visibility.Visible : Visibility.Collapsed;

    private void OnCommandKeyDown(object sender, KeyRoutedEventArgs e) {
        if (e.Key == VirtualKey.Enter && Console.SendCommand.CanExecute(null)) {
            Console.SendCommand.Execute(null);
            e.Handled = true;
        }
    }

    private async void OnBackupsClick(object sender, RoutedEventArgs e) {
        var dialog = new BackupsDialog(Server.Id, Server.Name, Server.IsRunning, DispatcherQueue) {
            XamlRoot = XamlRoot,
        };
        await dialog.ShowAsync();
    }

    private async void OnTitleRenameClick(object sender, RoutedEventArgs e) => await ShowRenameDialogAsync();

    private async void OnEditClick(object sender, RoutedEventArgs e) => await ShowEditDialogAsync();

    private async Task ShowEditDialogAsync() {
        var dialog = new EditServerDialog(_main, Server) { XamlRoot = XamlRoot };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            await _main.UpdateServerAsync(dialog.BuildRequest());
    }

    private async Task ShowRenameDialogAsync() {
        var nameBox = new TextBox {
            Text = Server.Name,
            Header = _localizer.Get("EditServerName_Header"),
            SelectionStart = 0,
            SelectionLength = Server.Name.Length,
        };
        var dialog = new ContentDialog {
            XamlRoot = XamlRoot,
            Title = _localizer.Get("RenameServerDialog_Title"),
            Content = nameBox,
            PrimaryButtonText = _localizer.Get("Common_Save"),
            CloseButtonText = _localizer.Get("Common_Cancel"),
            DefaultButton = ContentDialogButton.Primary,
        };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            await _main.RenameServerAsync(Server, nameBox.Text);
    }

    private async void OnOpenInstanceFolderClick(object sender, RoutedEventArgs e) {
        var folder = ResolveInstanceFolder();
        if (folder is null) {
            var dialog = new ContentDialog {
                XamlRoot = XamlRoot,
                Title = _localizer.Get("ServerFolder_NotFoundTitle"),
                Content = _localizer.Get("ServerFolder_NotFoundBody"),
                CloseButtonText = "OK",
                DefaultButton = ContentDialogButton.Close,
            };
            await dialog.ShowAsync();
            return;
        }

        Process.Start(new ProcessStartInfo {
            FileName = folder,
            UseShellExecute = true,
        });
    }

    private void OnPlayerOpClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "op \"{0}\"");
    private void OnPlayerDeopClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "deop \"{0}\"");
    private void OnPlayerKickClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "kick \"{0}\"");
    private void OnPlayerBanClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "ban \"{0}\"");
    private void OnPlayerRawClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "data get entity \"{0}\"");
    private void OnPlayerInventoryClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "data get entity \"{0}\" Inventory");

    private void SendPlayerCommand(object sender, string format) {
        var player = sender switch
        {
            FrameworkElement { Tag: PlayerItem taggedPlayer } => taggedPlayer.Name,
            FrameworkElement { DataContext: PlayerItem dataPlayer } => dataPlayer.Name,
            FrameworkElement { Tag: string taggedName } => taggedName,
            FrameworkElement { DataContext: string dataName } => dataName,
            _ => null,
        };
        if (string.IsNullOrWhiteSpace(player))
            return;

        Console.CommandText = string.Format(format, player.Replace("\"", "\\\""));
        if (Console.SendCommand.CanExecute(null))
            Console.SendCommand.Execute(null);
    }

    private string? ResolveInstanceFolder() {
        var roots = new[] {
            GetPackagedDataRoot(),
            Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "JustHostMC"),
        }.Where(p => !string.IsNullOrWhiteSpace(p)).Distinct(StringComparer.OrdinalIgnoreCase);

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
        }
        catch {
            return null;
        }
    }

    private async void OnDeleteClick(object sender, RoutedEventArgs e) {
        var confirm = new ContentDialog {
            XamlRoot = XamlRoot,
            Title = _localizer.Get("ServerDelete_Title"),
            Content = _localizer.Get("ServerDelete_Body"),
            PrimaryButtonText = _localizer.Get("ServerDelete_Confirm"),
            CloseButtonText = _localizer.Get("Common_Cancel"),
            DefaultButton = ContentDialogButton.Close,
        };
        if (await confirm.ShowAsync() != ContentDialogResult.Primary)
            return;
        _main.DeleteServerCommand.Execute(Server);
        _shell.RequestHome();
    }

    private async void OnUploadClick(object sender, RoutedEventArgs e) {
        var picker = new FileOpenPicker();
        picker.FileTypeFilter.Add(".jar");
        var hwnd = WinRT.Interop.WindowNative.GetWindowHandle(App.Current.MainWindow);
        WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);

        var file = await picker.PickSingleFileAsync();
        if (file is not null)
            await Mods.UploadAsync(file);
    }

    private async void OnRemoveModConfirm(object sender, RoutedEventArgs e) {
        if (sender is FrameworkElement { Tag: ModFileItem item })
            await Mods.RemoveAsync(item);
    }
}
