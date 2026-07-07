using System;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Grpc.Core;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>Reads and updates the log retention policy and runs an on-demand
/// purge.</summary>
public sealed partial class SettingsViewModel : ObservableObject {
    private const long BytesPerMb = 1024 * 1024;

    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;

    /// <summary>Number of days to keep logs (0 = no age limit).</summary>
    [ObservableProperty]
    public partial double KeepDays { get; set; }

    /// <summary>Total log size cap in megabytes (0 = no size limit).</summary>
    [ObservableProperty]
    public partial double MaxTotalMb {
        get; set;
    }

    [ObservableProperty]
    public partial string StatusMessage {
        get; private set;
    } = "";

    [ObservableProperty]
    public partial bool IsBusy {
        get; private set;
    }

    /// <summary>Localized description of the isolation backend currently in
    /// use.</summary>
    [ObservableProperty]
    public partial string ActiveModeText {
        get; private set;
    } = "";

    /// <summary>Localized Docker availability line.</summary>
    [ObservableProperty]
    public partial string DockerStatusText {
        get; private set;
    } = "";

    /// <summary>The user's Docker opt-in. Changing it persists immediately
    /// (effective next launch).</summary>
    [ObservableProperty]
    public partial bool UseDocker {
        get; set;
    }

    [ObservableProperty]
    public partial string AppVersionText {
        get; private set;
    } = "";

    public string AppName =>
        JustHostMC.App.Helpers.ProcessInfoHelper.ProductName;

    /// <summary>User-entered CurseForge API key (write-only; never read
    /// back).</summary>
    [ObservableProperty]
    public partial string CurseForgeKey { get; set; } = "";

    /// <summary>Localized line describing whether a CurseForge key is
    /// configured.</summary>
    [ObservableProperty]
    public partial string CurseForgeKeyStatus {
        get; private set;
    } = "";

    private bool _loadingBackend;

    public SettingsViewModel(DispatcherQueue dispatcher, ILocalizer localizer) {
        _dispatcher = dispatcher;
        _localizer  = localizer;

        AppVersionText = JustHostMC.App.Helpers.ProcessInfoHelper.FullVersion;
    }

    partial void OnUseDockerChanged(bool value) {
        if (!_loadingBackend)
            _ = ApplyUseDockerAsync(value);
    }

    /// <summary>Loads the current retention policy and backend info from the
    /// engine.</summary>
    public async Task LoadAsync() {
        try {
            var daemon = await App.Current.DaemonReady;
            var policy = await daemon.Settings.GetLogRetentionAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => {
                KeepDays   = policy.KeepDays;
                MaxTotalMb = policy.MaxTotalBytes / (double)BytesPerMb;
            });
        } catch (RpcException) {
            RunOnUI(() => StatusMessage =
                        _localizer.Get("Settings_LoadFailed"));
        }

        await LoadBackendAsync();
        await LoadShopKeysAsync();
    }

    private async Task LoadShopKeysAsync() {
        try {
            var daemon = await App.Current.DaemonReady;
            var keys   = await daemon.Settings.GetShopKeysAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
            var status = "Settings_ShopKeyNone";
            foreach (var key in keys.Keys) {
                if (key.ShopId != "curseforge")
                    continue;
                status = key.HasUserKey      ? "Settings_ShopKeyUser"
                         : key.HasBuiltinKey ? "Settings_ShopKeyBuiltin"
                                             : "Settings_ShopKeyNone";
            }
            RunOnUI(() => CurseForgeKeyStatus = _localizer.Get(status));
        } catch (RpcException) {
            // The shop key line stays empty; the rest of the page still works.
        }
    }

    [RelayCommand]
    private async Task SaveCurseForgeKey() {
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Settings.SetShopKeyAsync(
                new ShopKey {
                    ShopId = "curseforge",
                    ApiKey = CurseForgeKey,
                },
                deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => {
                CurseForgeKey = "";
                StatusMessage = _localizer.Get("Settings_Saved");
            });
            await LoadShopKeysAsync();
        } catch (RpcException) {
            RunOnUI(() => StatusMessage =
                        _localizer.Get("Settings_SaveFailed"));
        }
    }

    private async Task LoadBackendAsync() {
        try {
            var daemon = await App.Current.DaemonReady;
            var info   = await daemon.Settings.GetBackendInfoAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => {
                ActiveModeText = _localizer.Get(info.ActiveMode == "docker"
                                                    ? "Backend_Mode_Docker"
                                                    : "Backend_Mode_OnMachine");
                DockerStatusText =
                    info.DockerAvailable
                        ? _localizer.Get("Backend_DockerAvailable",
                                         ("version", info.DockerVersion))
                        : _localizer.Get("Backend_DockerUnavailable");
                _loadingBackend = true;
                UseDocker       = info.UseDocker;
                _loadingBackend = false;
            });
        } catch (RpcException) {
            // Leave backend fields at defaults; retention settings still work.
        }
    }

    private async Task ApplyUseDockerAsync(bool enabled) {
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Settings.SetUseDockerAsync(
                new UseDocker { Enabled = enabled },
                deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => StatusMessage =
                        _localizer.Get("Backend_DockerPrefSaved"));
        } catch (RpcException) {
            RunOnUI(() => StatusMessage =
                        _localizer.Get("Settings_SaveFailed"));
        }
    }

    [RelayCommand]
    private async Task Save() {
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Settings.SetLogRetentionAsync(
                new LogRetention {
                    KeepDays      = (int)Math.Max(0, KeepDays),
                    MaxTotalBytes = (long)Math.Max(0, MaxTotalMb) * BytesPerMb,
                },
                deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_Saved"));
        } catch (RpcException) {
            RunOnUI(() => StatusMessage =
                        _localizer.Get("Settings_SaveFailed"));
        }
    }

    [RelayCommand]
    private async Task PurgeNow() {
        RunOnUI(() => IsBusy = true);
        try {
            var daemon = await App.Current.DaemonReady;
            // Persist the current values first so the purge uses them.
            await Save();
            var result = await daemon.Settings.PurgeLogsAsync(
                new Empty(), deadline: DateTime.UtcNow.AddMinutes(2));
            RunOnUI(() => StatusMessage = _localizer.Get(
                        "Settings_PurgeResult",
                        ("count", result.RemovedFiles.ToString()),
                        ("size", FormatSize(result.FreedBytes))));
        } catch (RpcException) {
            RunOnUI(() => StatusMessage =
                        _localizer.Get("Settings_PurgeFailed"));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    [RelayCommand]
    private async Task RemoveAllData() {
        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = _localizer.Get("Settings_RemovingData");
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.RemoveAllDataAsync(
                new Empty(), deadline: DateTime.UtcNow.AddMinutes(2));
            RunOnUI(() => StatusMessage =
                        _localizer.Get("Settings_DataRemoved"));
        } catch (RpcException) {
            RunOnUI(() => StatusMessage =
                        _localizer.Get("Settings_RemoveDataFailed"));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    private static string FormatSize(long bytes) {
        string[] units = { "B", "KB", "MB", "GB", "TB" };
        double value   = bytes;
        var unit       = 0;
        while (value >= 1024 && unit < units.Length - 1) {
            value /= 1024;
            unit++;
        }
        return $"{value:0.#} {units[unit]}";
    }

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
