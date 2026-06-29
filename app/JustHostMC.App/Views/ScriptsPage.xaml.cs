using System;
using System.Collections.Generic;
using System.IO;
using System.Threading.Tasks;
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

/// <summary>Lets the user import their own Lua provider/automation scripts (with a
/// permission-consent step), and manage installed providers + automation scripts.</summary>
public sealed partial class ScriptsPage : Page
{
    private readonly ILocalizer _localizer = new LocalizationService();

    public ScriptsViewModel ViewModel { get; }

    public ScriptsPage()
    {
        NavigationCacheMode = NavigationCacheMode.Required;
        ViewModel = new ScriptsViewModel(DispatcherQueue, _localizer);
        InitializeComponent();
        Loaded += async (_, _) => await ViewModel.EnsureLoadedAsync();
    }

    private async void OnImportProviderClick(object sender, RoutedEventArgs e)
    {
        var lua = await PickLuaFileAsync();
        if (lua is null)
            return;

        string source;
        try
        {
            source = await FileIO.ReadTextAsync(lua);
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
        {
            ViewModel.StatusMessage = _localizer.Get("Scripts_ReadFailed");
            return;
        }

        // Optional bundled jar.
        var jarFile = await PickJarFileAsync();
        byte[]? jarBytes = null;
        string? jarName = null;
        if (jarFile is not null)
        {
            try
            {
                var buffer = await FileIO.ReadBufferAsync(jarFile);
                jarBytes = buffer.ToArray();
                jarName = jarFile.Name;
            }
            catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
            {
                ViewModel.StatusMessage = _localizer.Get("Scripts_ReadFailed");
                return;
            }
        }

        var granted = await RequestConsentAsync(lua.Name, source);
        if (granted is null)
            return; // cancelled

        await ViewModel.ImportProviderAsync(source, jarBytes, jarName, granted);
    }

    private async void OnImportScriptClick(object sender, RoutedEventArgs e)
    {
        var lua = await PickLuaFileAsync();
        if (lua is null)
            return;

        string source;
        try
        {
            source = await FileIO.ReadTextAsync(lua);
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
        {
            ViewModel.StatusMessage = _localizer.Get("Scripts_ReadFailed");
            return;
        }

        var granted = await RequestConsentAsync(lua.Name, source);
        if (granted is null)
            return;

        await ViewModel.ImportScriptAsync(source, granted);
    }

    /// <summary>Shows the consent dialog for the declared permissions parsed from
    /// the script. Returns the granted set, or null if the user cancelled.</summary>
    private async Task<IReadOnlyList<PermissionKind>?> RequestConsentAsync(string scriptName, string luaSource)
    {
        var permissions = LuaPermissions.Parse(luaSource);
        var dialog = new PermissionConsentDialog(scriptName, permissions, _localizer)
        {
            XamlRoot = XamlRoot,
        };
        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
            return null;
        return dialog.Granted;
    }

    private async void OnRemoveProviderClick(object sender, RoutedEventArgs e)
    {
        if (sender is not FrameworkElement { Tag: ProviderItem item })
            return;
        if (await ConfirmRemoveAsync(item.Name))
            await ViewModel.RemoveProviderAsync(item);
    }

    private async void OnRemoveScriptClick(object sender, RoutedEventArgs e)
    {
        if (sender is not FrameworkElement { Tag: ScriptItem item })
            return;
        if (await ConfirmRemoveAsync(item.Name))
            await ViewModel.RemoveScriptAsync(item);
    }

    private async void OnScriptToggled(object sender, RoutedEventArgs e)
    {
        if (sender is ToggleSwitch { Tag: ScriptItem item } toggle)
            await ViewModel.SetScriptEnabledAsync(item, toggle.IsOn);
    }

    private async Task<bool> ConfirmRemoveAsync(string name)
    {
        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            Title = _localizer.Get("Scripts_RemoveConfirmTitle"),
            Content = _localizer.Get("Scripts_RemoveConfirmBody", ("name", name)),
            PrimaryButtonText = _localizer.Get("Scripts_RemoveConfirmPrimary"),
            CloseButtonText = _localizer.Get("Scripts_RemoveConfirmCancel"),
            DefaultButton = ContentDialogButton.Close,
        };
        return await dialog.ShowAsync() == ContentDialogResult.Primary;
    }

    private static async Task<StorageFile?> PickLuaFileAsync()
    {
        var picker = new FileOpenPicker();
        picker.FileTypeFilter.Add(".lua");
        InitializeWithOwner(picker);
        return await picker.PickSingleFileAsync();
    }

    private static async Task<StorageFile?> PickJarFileAsync()
    {
        var picker = new FileOpenPicker();
        picker.FileTypeFilter.Add(".jar");
        InitializeWithOwner(picker);
        return await picker.PickSingleFileAsync();
    }

    private static void InitializeWithOwner(object picker)
    {
        // The app is unpackaged by default; FileOpenPicker needs the owner HWND.
        var hwnd = WinRT.Interop.WindowNative.GetWindowHandle(App.Current.MainWindow);
        WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);
    }
}
