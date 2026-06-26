using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using JustHostMC.App.Views;
using Microsoft.UI;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Data;
using System.Collections.ObjectModel;
using System.Collections.Specialized;
using System.Runtime.InteropServices;
using Windows.Graphics;

namespace JustHostMC.App;

public sealed partial class MainWindow : Window {
    private const int WmGetMinMaxInfo = 0x0024;
    private const int MinWindowDipWidth = 900;
    private const int MinWindowDipHeight = 640;
    private const nuint MinWindowSubclassId = 1;

    private delegate IntPtr SubclassProc(
        IntPtr hWnd,
        uint uMsg,
        IntPtr wParam,
        IntPtr lParam,
        nuint uIdSubclass,
        nuint dwRefData);

    [DllImport("user32.dll")]
    private static extern uint GetDpiForWindow(IntPtr hWnd);

    [DllImport("Comctl32.dll", SetLastError = true)]
    private static extern bool SetWindowSubclass(
        IntPtr hWnd,
        SubclassProc pfnSubclass,
        nuint uIdSubclass,
        nuint dwRefData);

    [DllImport("Comctl32.dll", SetLastError = true)]
    private static extern bool RemoveWindowSubclass(
        IntPtr hWnd,
        SubclassProc pfnSubclass,
        nuint uIdSubclass);

    [DllImport("Comctl32.dll")]
    private static extern IntPtr DefSubclassProc(IntPtr hWnd, uint uMsg, IntPtr wParam, IntPtr lParam);

    [StructLayout(LayoutKind.Sequential)]
    private struct Point
    {
        public int X;
        public int Y;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct MinMaxInfo
    {
        public Point Reserved;
        public Point MaxSize;
        public Point MaxPosition;
        public Point MinTrackSize;
        public Point MaxTrackSize;
    }

    private readonly Dictionary<string, NavigationViewItem> _serverItems = new();
    private readonly SubclassProc _subclassProc;
    private IntPtr _hwnd;
    private readonly NavigationViewItem HomeItem;

    /// <summary>The navigation shell: owns the shared MainViewModel.</summary>
    public NavShellViewModel Shell { get; }
    public ObservableCollection<object> MenuItems { get; } = new();

    public MainWindow() {
        _subclassProc = WindowSubclassProc;

        var localizer = new LocalizationService();
        Shell = new NavShellViewModel(new MainViewModel(localizer, DispatcherQueue));

        HomeItem = new NavigationViewItem {
            Tag = "home",
            Content = localizer.Get("NavHome/Content"),
            Icon = new FontIcon { Glyph = "\uE80F" }
        };

        InitializeComponent();
        MenuItems.Add(HomeItem);
        PaneFooterGrid.DataContext = Shell.Main.ProgressService;
        Title = localizer.Get("AppTitle");
        ExtendsContentIntoTitleBar = true;
        InstallMinimumWindowSizeHook();
        ResizeToContent(1200, 820);
        Closed += OnClosed;

        Shell.OpenServerRequested += OnOpenServerRequested;
        Shell.HomeRequested += () => Nav.SelectedItem = HomeItem;
        Shell.Main.Servers.CollectionChanged += OnServersChanged;

        ContentFrame.Loaded += (_, _) => {
            Nav.SelectedItem = HomeItem;
            _ = Shell.Main.ConnectAsync();
        };
    }

    private void ResizeToContent(int dipWidth, int dipHeight) {
        var hwnd = Win32Interop.GetWindowFromWindowId(AppWindow.Id);
        var scale = GetDpiForWindow(hwnd) / 96.0;
        AppWindow.Resize(new SizeInt32((int)(dipWidth * scale), (int)(dipHeight * scale)));
    }

    private void InstallMinimumWindowSizeHook() {
        _hwnd = Win32Interop.GetWindowFromWindowId(AppWindow.Id);
        if (_hwnd != IntPtr.Zero)
            SetWindowSubclass(_hwnd, _subclassProc, MinWindowSubclassId, 0);
    }

    private void OnClosed(object sender, WindowEventArgs args) {
        if (_hwnd != IntPtr.Zero)
            RemoveWindowSubclass(_hwnd, _subclassProc, MinWindowSubclassId);
    }

    private IntPtr WindowSubclassProc(
        IntPtr hWnd,
        uint uMsg,
        IntPtr wParam,
        IntPtr lParam,
        nuint uIdSubclass,
        nuint dwRefData) {
        if (uMsg == WmGetMinMaxInfo) {
            var scale = GetDpiForWindow(hWnd) / 96.0;
            var info = Marshal.PtrToStructure<MinMaxInfo>(lParam);
            info.MinTrackSize.X = Math.Max(info.MinTrackSize.X, (int)Math.Round(MinWindowDipWidth * scale));
            info.MinTrackSize.Y = Math.Max(info.MinTrackSize.Y, (int)Math.Round(MinWindowDipHeight * scale));
            Marshal.StructureToPtr(info, lParam, false);
            return IntPtr.Zero;
        }

        return DefSubclassProc(hWnd, uMsg, wParam, lParam);
    }

    private void OnServersChanged(object? sender, NotifyCollectionChangedEventArgs e) => SyncServerItems();

    /// <summary>Reconciles the sidebar's per-server entries with the live server list.</summary>
    private void SyncServerItems() {
        foreach (var server in Shell.Main.Servers) {
            if (_serverItems.ContainsKey(server.Id))
                continue;
            var item = CreateServerItem(server);
            _serverItems[server.Id] = item;
            MenuItems.Add(item);
        }

        var live = Shell.Main.Servers.Select(s => s.Id).ToHashSet();
        foreach (var (id, item) in _serverItems.Where(kv => !live.Contains(kv.Key)).ToList()) {
            MenuItems.Remove(item);
            _serverItems.Remove(id);
            _ = Shell.EvictServerCacheAsync(id);
        }

        var insertIndex = 1; // Home stays first.
        foreach (var server in Shell.Main.Servers) {
            if (!_serverItems.TryGetValue(server.Id, out var item))
                continue;

            var currentIndex = MenuItems.IndexOf(item);
            if (currentIndex >= 0 && currentIndex != insertIndex) {
                MenuItems.Move(currentIndex, insertIndex);
            }
            insertIndex++;
        }
    }

    private static NavigationViewItem CreateServerItem(ServerItem server) {
        var dot = new StatusDot { VerticalAlignment = VerticalAlignment.Center };
        dot.SetBinding(StatusDot.StatusProperty,
            new Binding { Source = server, Path = new PropertyPath("Status"), Mode = BindingMode.OneWay });

        var text = new TextBlock {
            VerticalAlignment = VerticalAlignment.Center,
            TextTrimming = TextTrimming.CharacterEllipsis,
        };
        text.SetBinding(TextBlock.TextProperty,
            new Binding { Source = server, Path = new PropertyPath("Name"), Mode = BindingMode.OneWay });

        var panel = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 10 };
        panel.Children.Add(dot);
        panel.Children.Add(text);

        var item = new NavigationViewItem { Content = panel, Tag = server };
        item.SetBinding(ToolTipService.ToolTipProperty,
            new Binding { Source = server.ProgressTracker, Path = new PropertyPath("TooltipText"), Mode = BindingMode.OneWay });

        return item;
    }

    private void OnNavSelectionChanged(NavigationView sender, NavigationViewSelectionChangedEventArgs args) {
        if (args.SelectedItem is not NavigationViewItem item)
            return;
        switch (item.Tag) {
            case "home":
                ContentFrame.Navigate(typeof(HomePage), Shell);
                break;
            case ServerItem server:
                ContentFrame.Navigate(typeof(ServerPage), new ServerPageArgs(server, Shell));
                break;
            case "settings":
                ContentFrame.Navigate(typeof(SettingsPage));
                break;
        }
    }

    private async void OnNavItemInvoked(NavigationView sender, NavigationViewItemInvokedEventArgs args) {
        if (args.InvokedItemContainer?.Tag as string == "add")
            await ShowCreateServerDialogAsync();
    }

    private void OnOpenServerRequested(ServerItem server) {
        if (_serverItems.TryGetValue(server.Id, out var item))
            Nav.SelectedItem = item;
    }

    private async System.Threading.Tasks.Task ShowCreateServerDialogAsync() {
        var dialog = new CreateServerDialog(Shell.Main) { XamlRoot = Content.XamlRoot };
        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
            return;
        if (dialog.BuildRequest() is { } request)
            await Shell.Main.InstallServerAsync(request);
    }
}
