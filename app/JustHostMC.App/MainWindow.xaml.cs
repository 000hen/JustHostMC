using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using JustHostMC.App.Views;
using McManager.Grpc;
using Microsoft.UI;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;
using System.Collections.Specialized;
using System.ComponentModel;
using System.Runtime.InteropServices;
using Windows.Graphics;

namespace JustHostMC.App;

public sealed partial class MainWindow : Window {
    private const int WmGetMinMaxInfo       = 0x0024;
    private const int MinWindowDipWidth     = 900;
    private const int MinWindowDipHeight    = 640;
    private const nuint MinWindowSubclassId = 1;

    private delegate IntPtr SubclassProc(IntPtr hWnd, uint uMsg, IntPtr wParam,
                                         IntPtr lParam, nuint uIdSubclass,
                                         nuint dwRefData);

    [DllImport("user32.dll")]
    private static extern uint GetDpiForWindow(IntPtr hWnd);

    [DllImport("Comctl32.dll", SetLastError = true)]
    private static extern bool SetWindowSubclass(IntPtr hWnd,
                                                 SubclassProc pfnSubclass,
                                                 nuint uIdSubclass,
                                                 nuint dwRefData);

    [DllImport("Comctl32.dll", SetLastError = true)]
    private static extern bool RemoveWindowSubclass(IntPtr hWnd,
                                                    SubclassProc pfnSubclass,
                                                    nuint uIdSubclass);

    [DllImport("Comctl32.dll")]
    private static extern IntPtr DefSubclassProc(IntPtr hWnd, uint uMsg,
                                                 IntPtr wParam, IntPtr lParam);

    [StructLayout(LayoutKind.Sequential)]
    private struct Point {
        public int X;
        public int Y;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct MinMaxInfo {
        public Point Reserved;
        public Point MaxSize;
        public Point MaxPosition;
        public Point MinTrackSize;
        public Point MaxTrackSize;
    }

    private readonly Dictionary<string, ServerItem> _trackedServers = new();
    private readonly Dictionary<string, ServerStatus> _lastServerStatuses =
        new();
    private readonly Queue<ServerTipNotification> _pendingServerTips = new();
    private readonly SubclassProc _subclassProc;
    private readonly ILocalizer _localizer;
    private ServerTipNotification? _currentServerTip;
    private IntPtr _hwnd;
    private long _paneFooterVisibilityCallbackToken;

    private enum ServerTipKind { Installed, Started, Stopped, Crashed }

    private sealed record ServerTipNotification(ServerItem Server,
                                                ServerProgressTracker? Tracker,
                                                ServerTipKind Kind);

    /// <summary>The navigation shell: owns the shared MainViewModel.</summary>
    public NavShellViewModel Shell { get; }

    public MainWindow() {
        _subclassProc = WindowSubclassProc;

        _localizer = new LocalizationService();
        Shell      = new NavShellViewModel(
            new MainViewModel(_localizer, DispatcherQueue));

        InitializeComponent();
        _paneFooterVisibilityCallbackToken =
            PaneFooterGrid.RegisterPropertyChangedCallback(
                UIElement.VisibilityProperty, OnPaneFooterVisibilityChanged);
        ServerStateTip.Closed += (_, _) => OnServerStateTipClosed();
        ServerStateTip.ActionButtonClick += (_, _) =>
            OnServerStateTipActionButtonClick();
        PaneFooterGrid.DataContext = Shell.Main.ProgressService;
        Title                      = _localizer.Get("AppTitle");
        ExtendsContentIntoTitleBar = true;
        SetTitleBar(SimpleTitleBar);
        InstallMinimumWindowSizeHook();
        ResizeToContent(1200, 820);
        Closed += OnClosed;

        Shell.OpenServerRequested += OnOpenServerRequested;
        Shell.HomeRequested += () => Nav.SelectedItem =
            NavigationDestination.Home;
        Shell.Main.Servers.CollectionChanged += OnServersChanged;

        ContentFrame.Loaded += (_, _) => {
            Nav.SelectedItem = NavigationDestination.Home;
            _                = Shell.Main.ConnectAsync();
        };
    }

    private void ResizeToContent(int dipWidth, int dipHeight) {
        var hwnd  = Win32Interop.GetWindowFromWindowId(AppWindow.Id);
        var scale = GetDpiForWindow(hwnd) / 96.0;
        AppWindow.Resize(
            new SizeInt32((int)(dipWidth * scale), (int)(dipHeight * scale)));
    }

    private void InstallMinimumWindowSizeHook() {
        _hwnd = Win32Interop.GetWindowFromWindowId(AppWindow.Id);
        if (_hwnd != IntPtr.Zero)
            SetWindowSubclass(_hwnd, _subclassProc, MinWindowSubclassId, 0);
    }

    private void OnClosed(object sender, WindowEventArgs args) {
        if (_hwnd != IntPtr.Zero)
            RemoveWindowSubclass(_hwnd, _subclassProc, MinWindowSubclassId);

        PaneFooterGrid.UnregisterPropertyChangedCallback(
            UIElement.VisibilityProperty, _paneFooterVisibilityCallbackToken);
        Shell.Main.Servers.CollectionChanged -= OnServersChanged;

        foreach (var server in _trackedServers.Values.ToList())
            UntrackServer(server);
    }

    private IntPtr WindowSubclassProc(IntPtr hWnd, uint uMsg, IntPtr wParam,
                                      IntPtr lParam, nuint uIdSubclass,
                                      nuint dwRefData) {
        if (uMsg == WmGetMinMaxInfo) {
            var scale = GetDpiForWindow(hWnd) / 96.0;
            var info  = Marshal.PtrToStructure<MinMaxInfo>(lParam);
            info.MinTrackSize.X =
                Math.Max(info.MinTrackSize.X,
                         (int)Math.Round(MinWindowDipWidth * scale));
            info.MinTrackSize.Y =
                Math.Max(info.MinTrackSize.Y,
                         (int)Math.Round(MinWindowDipHeight * scale));
            Marshal.StructureToPtr(info, lParam, false);
            return IntPtr.Zero;
        }

        return DefSubclassProc(hWnd, uMsg, wParam, lParam);
    }

    private void OnServersChanged(object? sender,
                                  NotifyCollectionChangedEventArgs e) {
        if (e.Action == NotifyCollectionChangedAction.Move)
            return;

        if (e.OldItems is not null) {
            foreach (var server in e.OldItems.OfType<ServerItem>()) {
                UntrackServer(server);
                _ = Shell.EvictServerCacheAsync(server.Id);
            }
        }

        if (e.NewItems is not null) {
            foreach (var server in e.NewItems.OfType<ServerItem>())
                TrackServer(server);
        }

        if (e.Action == NotifyCollectionChangedAction.Reset) {
            var live =
                Shell.Main.Servers.Select(server => server.Id).ToHashSet();
            foreach (var server in _trackedServers.Values
                         .Where(server => !live.Contains(server.Id))
                         .ToList()) {
                UntrackServer(server);
                _ = Shell.EvictServerCacheAsync(server.Id);
            }
            foreach (var server in Shell.Main.Servers) TrackServer(server);
        }
    }

    private void OnPaneFooterVisibilityChanged(DependencyObject sender,
                                               DependencyProperty property) {
        // NavigationView does not recalculate its menu ScrollViewer's cached
        // MaxHeight when PaneFooter content changes visibility. Wait for the
        // footer's new size, then nudge the PaneFooter property to invoke
        // NavigationView's own pane-layout update path.
        DispatcherQueue.TryEnqueue(() => {
            Nav.UpdateLayout();
            var footer = Nav.PaneFooter;
            if (footer is null)
                return;

            Nav.PaneFooter = null;
            Nav.PaneFooter = footer;
        });
    }

    private void TrackServer(ServerItem server) {
        if (!_trackedServers.TryAdd(server.Id, server))
            return;

        _lastServerStatuses[server.Id] = server.Status;
        server.PropertyChanged += OnTrackedServerPropertyChanged;
        server.ProgressTracker.PropertyChanged +=
            OnTrackedProgressPropertyChanged;

        if (server.ProgressTracker.IsReadyToRun)
            EnqueueServerTip(server, ServerTipKind.Installed,
                             server.ProgressTracker);
    }

    private void UntrackServer(ServerItem server) {
        if (!_trackedServers.Remove(server.Id))
            return;

        server.PropertyChanged -= OnTrackedServerPropertyChanged;
        server.ProgressTracker.PropertyChanged -=
            OnTrackedProgressPropertyChanged;
        _lastServerStatuses.Remove(server.Id);

        if (_currentServerTip?.Server == server)
            ServerStateTip.IsOpen = false;
    }

    private void OnTrackedServerPropertyChanged(object? sender,
                                                PropertyChangedEventArgs e) {
        if (sender is not ServerItem server ||
            !_trackedServers.TryGetValue(server.Id, out var trackedServer) ||
            !ReferenceEquals(server, trackedServer))
            return;

        if (e.PropertyName != nameof(ServerItem.Status))
            return;

        var previous =
            _lastServerStatuses.GetValueOrDefault(server.Id, server.Status);
        _lastServerStatuses[server.Id] = server.Status;

        if (server.Status == ServerStatus.Running &&
            previous != ServerStatus.Running)
            EnqueueServerTip(server, ServerTipKind.Started);
        else if (server.Status == ServerStatus.Stopped &&
                 previous is ServerStatus.Running or ServerStatus.Stopping)
            EnqueueServerTip(server, ServerTipKind.Stopped);
        else if (server.Status == ServerStatus.Crashed &&
                 previous != ServerStatus.Crashed)
            EnqueueServerTip(server, ServerTipKind.Crashed);
    }

    private void OnTrackedProgressPropertyChanged(object? sender,
                                                  PropertyChangedEventArgs e) {
        if (sender is not ServerProgressTracker tracker ||
            e.PropertyName != nameof(ServerProgressTracker.IsReadyToRun))
            return;

        var server = Shell.Main.Servers.FirstOrDefault(
            candidate => ReferenceEquals(candidate.ProgressTracker, tracker));
        if (tracker.IsReadyToRun && server is not null) {
            EnqueueServerTip(server, ServerTipKind.Installed, tracker);
        } else if (_currentServerTip
                   is { Kind : ServerTipKind.Installed } current &&
                   ReferenceEquals(current.Tracker, tracker)) {
            ServerStateTip.IsOpen = false;
        }
    }

    private void EnqueueServerTip(ServerItem server, ServerTipKind kind,
                                  ServerProgressTracker? tracker = null) {
        MarkServerStateChange(server);
        _pendingServerTips.Enqueue(
            new ServerTipNotification(server, tracker, kind));
        ShowNextServerTip();
    }

    private void MarkServerStateChange(ServerItem server) {
        if (!_trackedServers.ContainsKey(server.Id))
            return;

        server.HasUnreadStateChange =
            !ReferenceEquals(Nav.SelectedItem, server);
    }

    private void ShowNextServerTip() {
        if (_currentServerTip is not null || ServerStateTip.IsOpen)
            return;

        while (_pendingServerTips.TryDequeue(out var notification)) {
            if (!_trackedServers.ContainsKey(notification.Server.Id))
                continue;
            if (notification.Kind == ServerTipKind.Installed &&
                notification.Tracker?.IsReadyToRun != true)
                continue;

            Nav.UpdateLayout();
            if (Nav.ContainerFromMenuItem(notification.Server)
                    is not NavigationViewItem target)
                continue;

            _currentServerTip       = notification;
            ServerStateTip.Target   = target;
            ServerStateTip.Title    = TipTitle(notification);
            ServerStateTip.Subtitle = TipSubtitle(notification.Kind);
            ServerStateTip.ActionButtonContent =
                notification.Kind == ServerTipKind.Installed
                    ? _localizer.Get("ServerTeachingTip_StartAction")
                    : null;
            ServerStateTip.IsOpen = true;
            return;
        }
    }

    private string TipTitle(ServerTipNotification notification) =>
        _localizer.Get(
            notification.Kind switch {
                ServerTipKind.Installed => "ServerTeachingTip_InstalledTitle",
                ServerTipKind.Started   => "ServerTeachingTip_StartedTitle",
                ServerTipKind.Stopped   => "ServerTeachingTip_StoppedTitle",
                ServerTipKind.Crashed   => "ServerTeachingTip_CrashedTitle",
                _                       => "ServerStatus_Unknown",
            },
            ("server", notification.Server.Name));

    private string TipSubtitle(ServerTipKind kind) =>
        _localizer.Get(kind switch {
            ServerTipKind.Installed => "ServerTeachingTip_InstalledMessage",
            ServerTipKind.Started   => "ServerTeachingTip_StartedMessage",
            ServerTipKind.Stopped   => "ServerTeachingTip_StoppedMessage",
            ServerTipKind.Crashed   => "ServerTeachingTip_CrashedMessage",
            _                       => "ServerStatus_Unknown",
        });

    private void OnServerStateTipClosed() {
        var closed        = _currentServerTip;
        _currentServerTip = null;

        if (closed is { Kind : ServerTipKind.Installed, Tracker : not null })
            closed.Tracker.IsReadyToRun = false;

        DispatcherQueue.TryEnqueue(ShowNextServerTip);
    }

    private void OnServerStateTipActionButtonClick() {
        if (_currentServerTip
            is { Kind : ServerTipKind.Installed } notification &&
            Shell.Main.StartServerCommand.CanExecute(notification.Server)) {
            Shell.Main.StartServerCommand.Execute(notification.Server);
        }

        ServerStateTip.IsOpen = false;
    }

    private void OnNavSelectionChanged(
        NavigationView sender, NavigationViewSelectionChangedEventArgs args) {
        switch (args.SelectedItem) {
            case NavigationDestination.Home:
                if (ContentFrame.Content is not HomePage)
                    ContentFrame.Navigate(typeof(HomePage), Shell);
                break;
            case ServerItem server:
                server.HasUnreadStateChange = false;
                if (ContentFrame.Content is ServerPage page &&
                    page.Server == server)
                    break;
                ContentFrame.Navigate(typeof(ServerPage),
                                      new ServerPageArgs(server, Shell));
                break;
            case NavigationDestination.Scripts:
                if (ContentFrame.Content is not ScriptsPage)
                    ContentFrame.Navigate(typeof(ScriptsPage));
                break;
            case NavigationDestination.Settings:
                if (ContentFrame.Content is not SettingsPage)
                    ContentFrame.Navigate(typeof(SettingsPage));
                break;
        }
    }

    private void OnTitleBarBackRequested(TitleBar sender, object args) {
        if (ContentFrame.CanGoBack)
            ContentFrame.GoBack();
    }

    private void OnTitleBarPaneToggleRequested(TitleBar sender, object args) =>
        Nav.IsPaneOpen = !Nav.IsPaneOpen;

    private void OnContentFrameNavigated(object sender,
                                         NavigationEventArgs args) {
        object? item = args.SourcePageType switch {
            var pageType when pageType == typeof(HomePage) =>
                NavigationDestination.Home,
            var pageType when pageType == typeof(ScriptsPage) =>
                NavigationDestination.Scripts,
            var pageType when pageType == typeof(SettingsPage) =>
                NavigationDestination.Settings,
            var pageType when pageType == typeof(ServerPage) &&
                args.Parameter is ServerPageArgs serverArgs &&
                _trackedServers.TryGetValue(serverArgs.Server.Id,
                                            out var serverItem) => serverItem,
            _                                                   => null,
        };

        if (item is not null && !Equals(Nav.SelectedItem, item))
            Nav.SelectedItem = item;
    }

    private async void OnNavItemInvoked(
        NavigationView sender, NavigationViewItemInvokedEventArgs args) {
        if (args.InvokedItemContainer?.Tag as string == "add")
            await ShowCreateServerDialogAsync();
    }

    private void OnOpenServerRequested(ServerItem server) {
        if (_trackedServers.TryGetValue(server.Id, out var item))
            Nav.SelectedItem = item;
    }

    private async System.Threading.Tasks.Task ShowCreateServerDialogAsync() {
        var localizer = new LocalizationService();
        var content   = new ServerDialog(Shell.Main, ServerDialogMode.Create);
        var dialog    = new ContentDialog {
            XamlRoot = Content.XamlRoot,
            Style    = Application.Current
                           .Resources["DefaultContentDialogStyle"] as Style,
            Title    = localizer.Get("CreateServerDialog_Title"),
            Content  = content,
            PrimaryButtonText =
                localizer.Get("CreateServerDialog_PrimaryButtonText"),
            CloseButtonText =
                localizer.Get("CreateServerDialog_CloseButtonText"),
            DefaultButton          = ContentDialogButton.Primary,
            IsPrimaryButtonEnabled = content.CanSubmit,
        };
        content.CanSubmitChanged += (_, _) => dialog.IsPrimaryButtonEnabled =
            content.CanSubmit;
        ContentDialogSizing.Apply(dialog);

        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
            return;
        if (content.BuildCreateRequest() is {} request)
            await Shell.Main.InstallServerAsync(request);
    }
}
