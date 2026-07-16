using JustHostMC.App.Models;
using JustHostMC.App.ViewModels;
using JustHostMC.App.Views;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Windows.Storage.Pickers;

namespace JustHostMC.App.Controls.Server;

public sealed partial class ServerModsPanel : UserControl {
    public static readonly DependencyProperty ModsProperty =
        DependencyProperty.Register(
            nameof(Mods), typeof(ModsViewModel), typeof(ServerModsPanel),
            new PropertyMetadata(null, OnViewModelChanged));

    public static readonly DependencyProperty ServerProperty =
        DependencyProperty.Register(
            nameof(Server), typeof(ServerItem), typeof(ServerModsPanel),
            new PropertyMetadata(null, OnViewModelChanged));

    public ModsViewModel Mods {
        get => (ModsViewModel)GetValue(ModsProperty);
        set => SetValue(ModsProperty, value);
    }

    public ServerItem Server {
        get => (ServerItem)GetValue(ServerProperty);
        set => SetValue(ServerProperty, value);
    }

    public ServerModsPanel() {
        InitializeComponent();
    }

    private static void OnViewModelChanged(
        DependencyObject d, DependencyPropertyChangedEventArgs e) {
        var panel = (ServerModsPanel)d;
        panel.Bindings.Update();
    }

    private async void OnUploadClick(object sender, RoutedEventArgs e) {
        if (Mods == null)
            return;

        var picker = new FileOpenPicker();
        picker.FileTypeFilter.Add(".jar");
        if (Mods.AcceptsLiteMod)
            picker.FileTypeFilter.Add(".litemod");
        var hwnd =
            WinRT.Interop.WindowNative.GetWindowHandle(App.Current.MainWindow);
        WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);

        var files = await picker.PickMultipleFilesAsync();
        foreach (var file in files) await Mods.UploadAsync(file);
    }

    private async void OnExportModsClick(object sender, RoutedEventArgs e) {
        if (Mods == null || Server == null)
            return;

        var picker = new FileSavePicker();
        picker.FileTypeChoices.Add("ZIP", new List<string> { ".zip" });
        picker.SuggestedFileName = $"{Server.Name}-mods";
        var hwnd =
            WinRT.Interop.WindowNative.GetWindowHandle(App.Current.MainWindow);
        WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);

        var file = await picker.PickSaveFileAsync();
        if (file is not null)
            await Mods.ExportAllAsync(file.Path);
    }

    private void OnBrowseShopClick(object sender, RoutedEventArgs e) {
        if (Mods == null || Server == null)
            return;

        var context = new ShopContext(
            Server.Id, Server.McVersion, Server.ProviderId, Mods.Kind,
            Mods.InstalledFileNames, () => _ = Mods.RefreshAsync());
        new ShopWindow(context).Activate();
    }

    private async void OnRemoveModConfirm(object sender, RoutedEventArgs e) {
        if (Mods == null)
            return;

        if (sender is FrameworkElement { Tag : ModFileItem item })
            await Mods.RemoveAsync(item);
    }

    private async void OnModsScrollViewChanged(
        object sender, ScrollViewerViewChangedEventArgs e) {
        const double loadAheadThreshold = 240;
        if (Mods is not null && sender is ScrollViewer scrollViewer &&
            scrollViewer.ScrollableHeight - scrollViewer.VerticalOffset <=
                loadAheadThreshold) {
            await Mods.LoadMoreAsync();
        }
    }
}
