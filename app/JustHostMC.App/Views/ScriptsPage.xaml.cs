using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices.WindowsRuntime;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;
using Windows.Storage;
using Windows.Storage.Pickers;

namespace JustHostMC.App.Views;

/// <summary>Lets the user import their own Lua provider/automation scripts
/// (with a permission-consent step), and manage installed providers +
/// automation scripts.</summary>
public sealed partial class ScriptsPage : Page {
    private readonly ILocalizer _localizer = new LocalizationService();
    private ScriptLogsWindow? _logsWindow;

    public ScriptsViewModel ViewModel { get; }

    public ScriptsPage() {
        NavigationCacheMode = NavigationCacheMode.Required;
        ViewModel           = new ScriptsViewModel(DispatcherQueue, _localizer);
        InitializeComponent();
        Loaded += async (_, _) => await ViewModel.EnsureLoadedAsync();
    }

    private async void OnImportProviderClick(object sender, RoutedEventArgs e) {
        var lua = await PickLuaFileAsync();
        if (lua is null)
            return;

        string source;
        try {
            source = await FileIO.ReadTextAsync(lua);
        } catch (Exception ex)
            when (ex is IOException or UnauthorizedAccessException) {
            ViewModel.SetStatus(_localizer.Get("Scripts.ReadFailed"));
            return;
        }

        // Optional bundled jar.
        var jarFile      = await PickJarFileAsync();
        byte[]? jarBytes = null;
        string? jarName  = null;
        if (jarFile is not null) {
            try {
                var buffer = await FileIO.ReadBufferAsync(jarFile);
                jarBytes   = buffer.ToArray();
                jarName    = jarFile.Name;
            } catch (Exception ex)
                when (ex is IOException or UnauthorizedAccessException) {
                ViewModel.SetStatus(_localizer.Get("Scripts.ReadFailed"));
                return;
            }
        }

        var granted = await RequestConsentAsync(lua.Name, source);
        if (granted is null)
            return;  // cancelled

        await ViewModel.ImportProviderAsync(source, jarBytes, jarName, granted);
    }

    private async void OnImportScriptClick(object sender, RoutedEventArgs e) {
        var lua = await PickLuaFileAsync();
        if (lua is null)
            return;

        string source;
        try {
            source = await FileIO.ReadTextAsync(lua);
        } catch (Exception ex)
            when (ex is IOException or UnauthorizedAccessException) {
            ViewModel.SetStatus(_localizer.Get("Scripts.ReadFailed"));
            return;
        }

        var granted = await RequestConsentAsync(lua.Name, source);
        if (granted is null)
            return;

        await ViewModel.ImportScriptAsync(source, granted);
    }

    private async void OnImportParserClick(object sender, RoutedEventArgs e) {
        var lua = await PickLuaFileAsync();
        if (lua is null)
            return;

        string source;
        try {
            source = await FileIO.ReadTextAsync(lua);
        } catch (Exception ex)
            when (ex is IOException or UnauthorizedAccessException) {
            ViewModel.SetStatus(_localizer.Get("Scripts.ReadFailed"));
            return;
        }

        var granted = await RequestConsentAsync(lua.Name, source);
        if (granted is null)
            return;

        await ViewModel.ImportParserAsync(source, granted);
    }

    private void OnShowProvidersFolderClick(object sender, RoutedEventArgs e) =>
        ShowInFolder("providers");

    private void OnShowParsersFolderClick(object sender, RoutedEventArgs e) =>
        ShowInFolder("parsers");

    private void OnShowAutomationFolderClick(
        object sender, RoutedEventArgs e) => ShowInFolder("scripts");

    private void OnShowAllLogsClick(object sender, RoutedEventArgs e) {
        if (_logsWindow is null) {
            var window = new ScriptLogsWindow(ViewModel.LogSessions);
            window.Closed += (_, _) => {
                if (ReferenceEquals(_logsWindow, window))
                    _logsWindow = null;
            };
            _logsWindow = window;
        }

        _logsWindow.Activate();
    }

    private void ShowInFolder(string folderName) {
        try {
            var folder = Path.Combine(ResolveDataRoot(), folderName);
            Directory.CreateDirectory(folder);
            Process.Start(new ProcessStartInfo {
                FileName        = folder,
                UseShellExecute = true,
            });
        } catch (Exception ex)
            when (ex is IOException or UnauthorizedAccessException or
                      Win32Exception) {
            ViewModel.SetStatus(_localizer.Get("Scripts.OpenFolderFailed"));
        }
    }

    private static string ResolveDataRoot() {
        try {
            return ApplicationData.Current.LocalFolder.Path;
        } catch {
            return Path.Combine(
                Environment.GetFolderPath(
                    Environment.SpecialFolder.LocalApplicationData),
                "JustHostMC");
        }
    }

    /// <summary>Shows the consent dialog for the declared permissions parsed
    /// from the script. Returns the granted set, or null if the user
    /// cancelled.</summary>
    private async Task<IReadOnlyList<PermissionKind>?> RequestConsentAsync(
        string scriptName, string luaSource) {
        var permissions = LuaPermissions.Parse(luaSource);
        var content     = new PermissionConsentDialog(permissions, _localizer);
        var dialog = (ContentDialog)Resources["PermissionConsentHostDialog"];
        dialog.XamlRoot = XamlRoot;
        dialog.Title = _localizer.Get("PermissionConsentTitleNamed",
                                      ("name", scriptName));
        dialog.Content = content;
        ContentDialogSizing.Apply(dialog);

        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
            return null;
        return content.Granted;
    }

    private async void OnRemoveScript(object sender, RoutedEventArgs e) {
        if (sender is ScriptEntryCard { Item : ScriptItem item })
            await ViewModel.RemoveScriptAsync(item);
        else if (sender is ScriptEntryCard { Item : ProviderItem provider })
            await ViewModel.RemoveProviderAsync(provider);
        else if (sender is ScriptEntryCard { Item : ParserItem parser })
            await ViewModel.RemoveParserAsync(parser);
    }

    private async void OnScriptToggled(object sender, RoutedEventArgs e) {
        // ToggleSwitch raises Toggled when its IsOn binding is first applied
        // during template realization, so ignore events that match the known
        // state to avoid a load-time storm of (and a possible refresh loop
        // from) redundant RPCs.
        if (sender is ScriptEntryCard { Item : ScriptItem item } card &&
            card.ScriptEnabled != item.Enabled)
            await ViewModel.SetScriptEnabledAsync(item, card.ScriptEnabled);
    }

    private static async Task<StorageFile?> PickLuaFileAsync() {
        var picker = new FileOpenPicker();
        picker.FileTypeFilter.Add(".lua");
        InitializeWithOwner(picker);
        return await picker.PickSingleFileAsync();
    }

    private static async Task<StorageFile?> PickJarFileAsync() {
        var picker = new FileOpenPicker();
        picker.FileTypeFilter.Add(".jar");
        InitializeWithOwner(picker);
        return await picker.PickSingleFileAsync();
    }

    private static void InitializeWithOwner(object picker) {
        // The app is unpackaged by default; FileOpenPicker needs the owner
        // HWND.
        var hwnd =
            WinRT.Interop.WindowNative.GetWindowHandle(App.Current.MainWindow);
        WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);
    }
}
