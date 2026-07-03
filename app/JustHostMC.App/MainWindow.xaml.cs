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
using Microsoft.UI.Xaml.Navigation;
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

    /// <summary>The navigation shell: owns the shared MainViewModel.</summary>
    public NavShellViewModel Shell { get; }

    public MainWindow() {
        _subclassProc = WindowSubclassProc;

        var localizer = new LocalizationService();
        Shell = new NavShellViewModel(new MainViewModel(localizer, DispatcherQueue));

        InitializeComponent();
        PaneFooterGrid.DataContext = Shell.Main.ProgressService;
        Title = localizer.Get("AppTitle");
        ExtendsContentIntoTitleBar = true;
        SetTitleBar(SimpleTitleBar);
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
        var selectedItem = Nav.SelectedItem as NavigationViewItem;
        foreach (var server in Shell.Main.Servers) {
            if (_serverItems.ContainsKey(server.Id))
                continue;
            var item = CreateServerItem(server);
            _serverItems[server.Id] = item;
            Nav.MenuItems.Add(item);
        }

        var live = Shell.Main.Servers.Select(s => s.Id).ToHashSet();
        foreach (var (id, item) in _serverItems.Where(kv => !live.Contains(kv.Key)).ToList()) {
            Nav.MenuItems.Remove(item);
            _serverItems.Remove(id);
            _ = Shell.EvictServerCacheAsync(id);
        }

        var insertIndex = 1; // Home stays first.
        foreach (var server in Shell.Main.Servers) {
            if (!_serverItems.TryGetValue(server.Id, out var item))
                continue;

            var currentIndex = Nav.MenuItems.IndexOf(item);
            if (currentIndex >= 0 && currentIndex != insertIndex) {
                Nav.MenuItems.RemoveAt(currentIndex);
                Nav.MenuItems.Insert(insertIndex, item);
            }
            insertIndex++;
        }

        if (selectedItem != null
            && !Equals(Nav.SelectedItem, selectedItem)
            && (Nav.MenuItems.Contains(selectedItem) || Nav.FooterMenuItems.Contains(selectedItem))) {
            Nav.SelectedItem = selectedItem;
        }

        Nav.IsTitleBarAutoPaddingEnabled = true;
        Nav.IsTitleBarAutoPaddingEnabled = false;
        Nav.UpdateLayout();
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
                if (ContentFrame.Content is not HomePage)
                    ContentFrame.Navigate(typeof(HomePage), Shell);
                break;
            case ServerItem server:
                if (ContentFrame.Content is ServerPage page && page.Server == server)
                    break;
                ContentFrame.Navigate(typeof(ServerPage), new ServerPageArgs(server, Shell));
                break;
            case "scripts":
                if (ContentFrame.Content is not ScriptsPage)
                    ContentFrame.Navigate(typeof(ScriptsPage));
                break;
            case "settings":
                if (ContentFrame.Content is not SettingsPage)
                    ContentFrame.Navigate(typeof(SettingsPage));
                break;
        }
    }

    private void OnTitleBarBackRequested(TitleBar sender, object args) {
        if (ContentFrame.CanGoBack)
            ContentFrame.GoBack();
    }

    private void OnTitleBarPaneToggleRequested(TitleBar sender, object args)
        => Nav.IsPaneOpen = !Nav.IsPaneOpen;

    private void OnContentFrameNavigated(object sender, NavigationEventArgs args) {
        NavigationViewItem? item = args.SourcePageType switch {
            var pageType when pageType == typeof(HomePage) => HomeItem,
            var pageType when pageType == typeof(ScriptsPage) => ScriptsItem,
            var pageType when pageType == typeof(SettingsPage) => SettingsItem,
            var pageType when pageType == typeof(ServerPage)
                && args.Parameter is ServerPageArgs serverArgs
                && _serverItems.TryGetValue(serverArgs.Server.Id, out var serverItem) => serverItem,
            _ => null,
        };

        if (item is not null && !Equals(Nav.SelectedItem, item))
            Nav.SelectedItem = item;
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
        var localizer = new LocalizationService();
        var content = new CreateServerDialog(Shell.Main);
        var dialog = new ContentDialog {
            XamlRoot = Content.XamlRoot,
            Style = Application.Current.Resources["DefaultContentDialogStyle"] as Style,
            Title = localizer.Get("CreateServerDialog_Title"),
            Content = content,
            PrimaryButtonText = localizer.Get("CreateServerDialog_PrimaryButtonText"),
            CloseButtonText = localizer.Get("CreateServerDialog_CloseButtonText"),
            DefaultButton = ContentDialogButton.Primary,
            IsPrimaryButtonEnabled = content.CanSubmit,
        };
        content.CanSubmitChanged += (_, _) => dialog.IsPrimaryButtonEnabled = content.CanSubmit;
        ContentDialogSizing.Apply(dialog);

        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
            return;
        if (content.BuildRequest() is { } request)
            await Shell.Main.InstallServerAsync(request);
    }
}
