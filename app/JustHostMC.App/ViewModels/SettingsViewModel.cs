using System;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Grpc.Core;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>Reads and updates the log retention policy and runs an on-demand purge.</summary>
public sealed partial class SettingsViewModel : ObservableObject
{
    private const long BytesPerMb = 1024 * 1024;

    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;

    private double _keepDays;
    private double _maxTotalMb;
    private string _statusMessage = "";
    private bool _isBusy;

    private string _activeModeText = "";
    private string _dockerStatusText = "";
    private bool _useDocker;
    private bool _loadingBackend;

    public SettingsViewModel(DispatcherQueue dispatcher, ILocalizer localizer)
    {
        _dispatcher = dispatcher;
        _localizer = localizer;
    }

    /// <summary>Number of days to keep logs (0 = no age limit).</summary>
    public double KeepDays
    {
        get => _keepDays;
        set => SetProperty(ref _keepDays, value);
    }

    /// <summary>Total log size cap in megabytes (0 = no size limit).</summary>
    public double MaxTotalMb
    {
        get => _maxTotalMb;
        set => SetProperty(ref _maxTotalMb, value);
    }

    public string StatusMessage
    {
        get => _statusMessage;
        private set => SetProperty(ref _statusMessage, value);
    }

    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    /// <summary>Localized description of the isolation backend currently in use.</summary>
    public string ActiveModeText
    {
        get => _activeModeText;
        private set => SetProperty(ref _activeModeText, value);
    }

    /// <summary>Localized Docker availability line.</summary>
    public string DockerStatusText
    {
        get => _dockerStatusText;
        private set => SetProperty(ref _dockerStatusText, value);
    }

    /// <summary>The user's Docker opt-in. Changing it persists immediately (effective next launch).</summary>
    public bool UseDocker
    {
        get => _useDocker;
        set
        {
            if (SetProperty(ref _useDocker, value) && !_loadingBackend)
                _ = ApplyUseDockerAsync(value);
        }
    }

    /// <summary>Loads the current retention policy and backend info from the engine.</summary>
    public async Task LoadAsync()
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            var policy = await daemon.Settings.GetLogRetentionAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() =>
            {
                KeepDays = policy.KeepDays;
                MaxTotalMb = policy.MaxTotalBytes / (double)BytesPerMb;
            });
        }
        catch (RpcException)
        {
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_LoadFailed"));
        }

        await LoadBackendAsync();
    }

    private async Task LoadBackendAsync()
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            var info = await daemon.Settings.GetBackendInfoAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() =>
            {
                ActiveModeText = _localizer.Get(info.ActiveMode == "docker"
                    ? "Backend_Mode_Docker"
                    : "Backend_Mode_OnMachine");
                DockerStatusText = info.DockerAvailable
                    ? _localizer.Get("Backend_DockerAvailable", ("version", info.DockerVersion))
                    : _localizer.Get("Backend_DockerUnavailable");
                _loadingBackend = true;
                UseDocker = info.UseDocker;
                _loadingBackend = false;
            });
        }
        catch (RpcException)
        {
            // Leave backend fields at defaults; retention settings still work.
        }
    }

    private async Task ApplyUseDockerAsync(bool enabled)
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Settings.SetUseDockerAsync(
                new UseDocker { Enabled = enabled }, deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => StatusMessage = _localizer.Get("Backend_DockerPrefSaved"));
        }
        catch (RpcException)
        {
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_SaveFailed"));
        }
    }

    [RelayCommand]
    private async Task Save()
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Settings.SetLogRetentionAsync(new LogRetention
            {
                KeepDays = (int)Math.Max(0, KeepDays),
                MaxTotalBytes = (long)Math.Max(0, MaxTotalMb) * BytesPerMb,
            }, deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_Saved"));
        }
        catch (RpcException)
        {
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_SaveFailed"));
        }
    }

    [RelayCommand]
    private async Task PurgeNow()
    {
        RunOnUI(() => IsBusy = true);
        try
        {
            var daemon = await App.Current.DaemonReady;
            // Persist the current values first so the purge uses them.
            await Save();
            var result = await daemon.Settings.PurgeLogsAsync(
                new Empty(), deadline: DateTime.UtcNow.AddMinutes(2));
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_PurgeResult",
                ("count", result.RemovedFiles.ToString()),
                ("size", FormatSize(result.FreedBytes))));
        }
        catch (RpcException)
        {
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_PurgeFailed"));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    [RelayCommand]
    private async Task RemoveAllData()
    {
        RunOnUI(() =>
        {
            IsBusy = true;
            StatusMessage = _localizer.Get("Settings_RemovingData");
        });
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.RemoveAllDataAsync(new Empty(), deadline: DateTime.UtcNow.AddMinutes(2));
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_DataRemoved"));
        }
        catch (RpcException)
        {
            RunOnUI(() => StatusMessage = _localizer.Get("Settings_RemoveDataFailed"));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    private static string FormatSize(long bytes)
    {
        string[] units = { "B", "KB", "MB", "GB", "TB" };
        double value = bytes;
        var unit = 0;
        while (value >= 1024 && unit < units.Length - 1)
        {
            value /= 1024;
            unit++;
        }
        return $"{value:0.#} {units[unit]}";
    }

    private void RunOnUI(Action action)
    {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
