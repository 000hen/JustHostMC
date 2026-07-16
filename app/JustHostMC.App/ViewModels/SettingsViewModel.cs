using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Grpc.Core;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

public enum SettingsWorkflowStatus {
    None,
    LoadFailed,
    Saved,
    SaveFailed,
    DockerPreferenceSaved,
    PurgeFailed,
    RemovingData,
    DataRemoved,
    RemoveDataFailed,
    RemovingIncompleteInstallations,
    RemoveIncompleteInstallationsFailed,
}

public enum ShopKeyConfiguration {
    Unknown,
    None,
    User,
    Builtin,
}

public enum BackendMode {
    Unknown,
    OnMachine,
    Docker,
}

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
    public partial string DynamicStatusMessage {
        get; private set;
    } = "";

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(IsLoadFailedStatus))]
    [NotifyPropertyChangedFor(nameof(IsSavedStatus))]
    [NotifyPropertyChangedFor(nameof(IsSaveFailedStatus))]
    [NotifyPropertyChangedFor(nameof(IsDockerPreferenceSavedStatus))]
    [NotifyPropertyChangedFor(nameof(IsPurgeFailedStatus))]
    [NotifyPropertyChangedFor(nameof(IsRemovingDataStatus))]
    [NotifyPropertyChangedFor(nameof(IsDataRemovedStatus))]
    [NotifyPropertyChangedFor(nameof(IsRemoveDataFailedStatus))]
    [NotifyPropertyChangedFor(nameof(IsRemovingIncompleteStatus))]
    [NotifyPropertyChangedFor(nameof(IsRemoveIncompleteFailedStatus))]
    public partial SettingsWorkflowStatus WorkflowStatus {
        get; private set;
    }

    public bool IsLoadFailedStatus =>
        WorkflowStatus == SettingsWorkflowStatus.LoadFailed;
    public bool IsSavedStatus => WorkflowStatus == SettingsWorkflowStatus.Saved;
    public bool IsSaveFailedStatus =>
        WorkflowStatus == SettingsWorkflowStatus.SaveFailed;
    public bool IsDockerPreferenceSavedStatus =>
        WorkflowStatus == SettingsWorkflowStatus.DockerPreferenceSaved;
    public bool IsPurgeFailedStatus =>
        WorkflowStatus == SettingsWorkflowStatus.PurgeFailed;
    public bool IsRemovingDataStatus =>
        WorkflowStatus == SettingsWorkflowStatus.RemovingData;
    public bool IsDataRemovedStatus =>
        WorkflowStatus == SettingsWorkflowStatus.DataRemoved;
    public bool IsRemoveDataFailedStatus =>
        WorkflowStatus == SettingsWorkflowStatus.RemoveDataFailed;
    public bool IsRemovingIncompleteStatus =>
        WorkflowStatus ==
        SettingsWorkflowStatus.RemovingIncompleteInstallations;
    public bool IsRemoveIncompleteFailedStatus =>
        WorkflowStatus ==
        SettingsWorkflowStatus.RemoveIncompleteInstallationsFailed;

    [ObservableProperty]
    public partial bool IsBusy { get; private set; }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(HasIncompleteInstallations))]
    [NotifyPropertyChangedFor(nameof(IncompleteInstallationDescription))]
    public partial int IncompleteInstallationCount {
        get; private set;
    }

    public bool HasIncompleteInstallations => IncompleteInstallationCount > 0;
    public string IncompleteInstallationDescription =>
        _localizer.Get("Settings_IncompleteInstallationsDescription",
                       ("count", IncompleteInstallationCount.ToString()));

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(IsDockerMode))]
    [NotifyPropertyChangedFor(nameof(IsOnMachineMode))]
    [NotifyPropertyChangedFor(nameof(IsDockerUnavailable))]
    public partial BackendMode ActiveBackendMode { get; private set; }

    public bool IsDockerMode    => ActiveBackendMode == BackendMode.Docker;
    public bool IsOnMachineMode => ActiveBackendMode == BackendMode.OnMachine;

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(IsDockerUnavailable))]
    public partial bool IsDockerAvailable { get; private set; }

    public bool IsDockerUnavailable =>
        ActiveBackendMode != BackendMode.Unknown && !IsDockerAvailable;

    /// <summary>Translator-controlled version format for the available case.
    /// The unavailable finite label is owned by XAML.</summary>
    [ObservableProperty]
    public partial string DockerAvailableText { get; private set; } = "";

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

    private bool _loadingBackend;
    private bool _isLoadingLogs;

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
                _isLoadingLogs = true;
                KeepDays       = policy.KeepDays;
                MaxTotalMb     = policy.MaxTotalBytes / (double)BytesPerMb;
                _isLoadingLogs = false;
            });
        } catch (RpcException) {
            RunOnUI(() => SetWorkflowStatus(SettingsWorkflowStatus.LoadFailed));
        }

        await LoadBackendAsync();
        await RefreshIncompleteInstallationsAsync();
    }

    private async Task RefreshIncompleteInstallationsAsync() {
        try {
            var daemon = await App.Current.DaemonReady;
            var list   = await daemon.Servers.ListAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
            var count = list.Servers.Count(server => server.Status ==
                                                     ServerStatus.Installing);
            RunOnUI(() => IncompleteInstallationCount = count);
        } catch (RpcException) {
            // Leave the cleanup action disabled until the next page load.
        }
    }

    }

    private async Task LoadBackendAsync() {
        try {
            var daemon = await App.Current.DaemonReady;
            var info   = await daemon.Settings.GetBackendInfoAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => {
                ActiveBackendMode = info.ActiveMode == "docker"
                                        ? BackendMode.Docker
                                        : BackendMode.OnMachine;
                IsDockerAvailable = info.DockerAvailable;
                DockerAvailableText =
                    info.DockerAvailable
                        ? _localizer.Get("Backend_DockerAvailable",
                                         ("version", info.DockerVersion))
                        : "";
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
            RunOnUI(() => SetWorkflowStatus(
                        SettingsWorkflowStatus.DockerPreferenceSaved));
        } catch (RpcException) {
            RunOnUI(() => SetWorkflowStatus(SettingsWorkflowStatus.SaveFailed));
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
        } catch (RpcException) {
            // Silently fail or log it
        }
    }

    partial void OnKeepDaysChanged(double value) {
        if (!_isLoadingLogs) {
            _ = Save();
        }
    }

    partial void OnMaxTotalMbChanged(double value) {
        if (!_isLoadingLogs) {
            _ = Save();
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
            RunOnUI(() => SetDynamicStatus(_localizer.Get(
                        "Settings_PurgeResult",
                        ("count", result.RemovedFiles.ToString()),
                        ("size", FormatSize(result.FreedBytes)))));
        } catch (RpcException) {
            RunOnUI(() =>
                        SetWorkflowStatus(SettingsWorkflowStatus.PurgeFailed));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    [RelayCommand]
    private async Task RemoveAllData() {
        RunOnUI(() => {
            IsBusy = true;
            SetWorkflowStatus(SettingsWorkflowStatus.RemovingData);
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.RemoveAllDataAsync(
                new Empty(), deadline: DateTime.UtcNow.AddMinutes(2));
            RunOnUI(() =>
                        SetWorkflowStatus(SettingsWorkflowStatus.DataRemoved));
        } catch (RpcException) {
            RunOnUI(() => SetWorkflowStatus(
                        SettingsWorkflowStatus.RemoveDataFailed));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    [RelayCommand]
    private async Task RemoveIncompleteInstallations() {
        RunOnUI(() => {
            IsBusy = true;
            SetWorkflowStatus(
                SettingsWorkflowStatus.RemovingIncompleteInstallations);
        });
        try {
            var daemon = await App.Current.DaemonReady;
            var list   = await daemon.Servers.ListAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
            var incomplete =
                list.Servers
                    .Where(server => server.Status == ServerStatus.Installing)
                    .ToArray();
            foreach (var server in incomplete) {
                await daemon.Servers.DeleteAsync(
                    new ServerId { Id = server.Id },
                    deadline: DateTime.UtcNow.AddMinutes(2));
            }
            RunOnUI(() => SetDynamicStatus(_localizer.Get(
                        "Settings_IncompleteInstallationsRemoved",
                        ("count", incomplete.Length.ToString()))));
        } catch (RpcException) {
            RunOnUI(() => SetWorkflowStatus(
                        SettingsWorkflowStatus
                            .RemoveIncompleteInstallationsFailed));
        } finally {
            await RefreshIncompleteInstallationsAsync();
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

    private void SetWorkflowStatus(SettingsWorkflowStatus status) {
        DynamicStatusMessage = "";
        WorkflowStatus       = status;
    }

    private void SetDynamicStatus(string message) {
        WorkflowStatus       = SettingsWorkflowStatus.None;
        DynamicStatusMessage = message;
    }

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
