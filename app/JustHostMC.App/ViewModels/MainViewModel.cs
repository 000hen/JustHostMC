using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Linq;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>
/// Backs the main window: connects to the engine, lists/creates/starts/stops
/// servers, and surfaces install progress. All user-visible text is localized and
/// every gRPC stream/callback update is marshaled to the UI thread (PROMPT §10.1).
/// </summary>
public partial class MainViewModel : ObservableObject
{
    private const int MaxLogLines = 2000;
    private const int MaxLogLineLength = 2000;

    private readonly ILocalizer _localizer;
    private readonly DispatcherQueue _dispatcher;
    private readonly object _pendingUpdatesGate = new();
    private readonly Dictionary<string, UpdateServerRequest> _pendingUpdates = new();
    private DispatcherQueueTimer? _refreshTimer;

    public ObservableCollection<ServerItem> Servers { get; } = new();
    public ObservableCollection<string> InstallLog { get; } = new();
    public ServerProgressService ProgressService { get; }

    public int TotalServers => Servers.Count;
    public int RunningServers => Servers.Count(s => s.Status == ServerStatus.Running);
    public int StoppedServers => Servers.Count(s => s.Status is ServerStatus.Stopped or ServerStatus.Crashed);
    public int BusyServers => Servers.Count(s => s.Status is ServerStatus.Installing or ServerStatus.Starting or ServerStatus.Stopping);

    private string _engineStatus;
    public string EngineStatus
    {
        get => _engineStatus;
        private set => SetProperty(ref _engineStatus, value);
    }

    private bool _isInstalling;
    public bool IsInstalling
    {
        get => _isInstalling;
        private set => SetProperty(ref _isInstalling, value);
    }

    private bool _installFailed;
    public bool InstallFailed
    {
        get => _installFailed;
        private set => SetProperty(ref _installFailed, value);
    }

    private string _installStep = "";
    public string InstallStep
    {
        get => _installStep;
        private set => SetProperty(ref _installStep, value);
    }

    private double _installFraction;
    public double InstallFraction
    {
        get => _installFraction;
        private set => SetProperty(ref _installFraction, value);
    }

    private bool _installIsIndeterminate = true;
    public bool InstallIsIndeterminate
    {
        get => _installIsIndeterminate;
        private set => SetProperty(ref _installIsIndeterminate, value);
    }

    public MainViewModel(ILocalizer localizer, DispatcherQueue dispatcher)
    {
        _localizer = localizer;
        _dispatcher = dispatcher;
        ProgressService = new ServerProgressService(_dispatcher);
        _engineStatus = _localizer.Get("EngineStatus_Connecting");
    }

    /// <summary>Waits for the engine, probes Health, loads servers, polls status.</summary>
    public async Task ConnectAsync()
    {
        RunOnUI(() => EngineStatus = _localizer.Get("EngineStatus_Connecting"));
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Engine.HealthAsync(new Empty(), deadline: DateTime.UtcNow.AddSeconds(10));
            RunOnUI(() => EngineStatus = _localizer.Get("EngineStatus_Connected"));
            await RefreshAsync();
            StartAutoRefresh();
        }
        catch (Exception)
        {
            RunOnUI(() => EngineStatus = _localizer.Get("EngineStatus_Failed"));
        }
    }

    /// <summary>Fetches available versions for a provider (for the create wizard).</summary>
    public async Task<string[]> GetVersionsAsync(string providerId)
    {
        var daemon = await App.Current.DaemonReady;
        var list = await daemon.Engine.ListVersionsAsync(
            new VersionQuery { ProviderId = providerId }, deadline: DateTime.UtcNow.AddSeconds(30));
        return list.Versions.ToArray();
    }

    /// <summary>Fetches the installed provider scripts (built-in + user-imported).</summary>
    public async Task<IReadOnlyList<ProviderInfo>> GetProvidersAsync()
    {
        var daemon = await App.Current.DaemonReady;
        var list = await daemon.Providers.ListAsync(
            new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
        return list.Providers;
    }

    /// <summary>
    /// Suggests a unique default server name (e.g. "My Server", then "My Server (1)")
    /// for when the user creates a server without typing one.
    /// </summary>
    public string SuggestDefaultServerName()
    {
        var baseName = _localizer.Get("CreateServer_DefaultName");
        bool Taken(string n) => Servers.Any(s => string.Equals(s.Name, n, StringComparison.OrdinalIgnoreCase));
        if (!Taken(baseName))
            return baseName;
        for (var i = 1; ; i++)
        {
            var candidate = $"{baseName} ({i})";
            if (!Taken(candidate))
                return candidate;
        }
    }

    [RelayCommand]
    private Task Refresh() => RefreshAsync();

    private async Task RefreshAsync()
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            var list = await daemon.Servers.ListAsync(new Empty(), deadline: DateTime.UtcNow.AddSeconds(10))
                .ConfigureAwait(false);
            RunOnUI(() => MergeServers(list.Servers));
        }
        catch (RpcException)
        {
            // Transient; the next poll will reconcile.
        }
    }

    /// <summary>Creates a server, streaming localized install progress + raw log.</summary>
    public async Task InstallServerAsync(CreateServerRequest request)
    {
        var tracker = ProgressService.GetOrCreateTracker(null, request.Name);
        RunOnUI(() =>
        {
            tracker.InstallLog.Clear();
            tracker.HasFailed = false;
            tracker.IsInstalling = true;
            tracker.IsActive = true;
            tracker.IsReadyToRun = false;
            tracker.IsIndeterminate = true;
            tracker.ProgressFraction = 0;
            tracker.CurrentStep = _localizer.Get("install_progress_preparing");

            InstallLog.Clear();
            InstallFailed = false;
            IsInstalling = true;
            InstallIsIndeterminate = true;
            InstallFraction = 0;
            InstallStep = _localizer.Get("install_progress_preparing");
        });

        try
        {
            var daemon = await App.Current.DaemonReady;
            using var call = daemon.Servers.Create(request);
            await foreach (var progress in call.ResponseStream.ReadAllAsync().ConfigureAwait(false))
            {
                var snapshot = progress;
                RunOnUI(() =>
                {
                    ApplyInstallProgress(snapshot);
                    if (snapshot.Step is { Key.Length: > 0 } step)
                        tracker.CurrentStep = _localizer.Get(step.Key);
                    if (snapshot.Fraction >= 0)
                    {
                        tracker.IsIndeterminate = false;
                        tracker.ProgressFraction = snapshot.Fraction;
                    }
                    else
                    {
                        tracker.IsIndeterminate = true;
                    }
                    if (!string.IsNullOrEmpty(snapshot.LogLine))
                        tracker.AppendLog(snapshot.LogLine);
                });
            }
            await RefreshAsync();
            RunOnUI(() =>
            {
                IsInstalling = false;
                tracker.IsInstalling = false;
                tracker.IsActive = false;
                tracker.IsReadyToRun = true;
                tracker.CurrentStep = _localizer.Get("install_progress_done") + " " + _localizer.Get("install_ready_to_run");
            });
        }
        catch (RpcException ex)
        {
            var key = MapErrorKey(ex);
            var detail = ex.Status.Detail;
            RunOnUI(() =>
            {
                InstallFailed = true;
                InstallIsIndeterminate = false;
                InstallStep = string.IsNullOrEmpty(detail)
                    ? _localizer.Get(key)
                    : $"{_localizer.Get(key)}: {detail}";

                tracker.HasFailed = true;
                tracker.IsIndeterminate = false;
                tracker.IsActive = false;
                tracker.CurrentStep = InstallStep;
            });
        }
    }

    public async Task<bool> UpdateServerAsync(UpdateServerRequest request)
    {
        var item = Servers.FirstOrDefault(s => s.Id == request.Id);
        var rollback = item is null ? null : BuildUpdateRequest(item);
        var optimistic = CloneUpdateRequest(request);
        lock (_pendingUpdatesGate)
            _pendingUpdates[optimistic.Id] = optimistic;
        if (item is not null)
            RunOnUI(() => item.ApplyLocal(optimistic));

        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.UpdateAsync(optimistic, deadline: DateTime.UtcNow.AddMinutes(3));
            lock (_pendingUpdatesGate)
                _pendingUpdates.Remove(optimistic.Id);
            await RefreshAsync();
            return true;
        }
        catch (RpcException)
        {
            lock (_pendingUpdatesGate)
                _pendingUpdates.Remove(optimistic.Id);
            if (item is not null && rollback is not null)
                RunOnUI(() => item.ApplyLocal(rollback));
            await RefreshAsync();
            return false;
        }
    }

    public Task<bool> RenameServerAsync(ServerItem item, string name)
    {
        name = name.Trim();
        if (name.Length == 0)
            return Task.FromResult(false);

        var request = BuildUpdateRequest(item);
        request.Name = name;
        return UpdateServerAsync(request);
    }

    public async Task MoveServerAsync(ServerItem item, int offset)
    {
        var ordered = Servers.ToList();
        var index = ordered.FindIndex(s => s.Id == item.Id);
        var target = index + offset;
        if (index < 0 || target < 0 || target >= ordered.Count)
            return;

        ordered.RemoveAt(index);
        ordered.Insert(target, item);

        for (var i = 0; i < ordered.Count; i++)
        {
            var request = BuildUpdateRequest(ordered[i]);
            request.SortOrder = i;
            if (!await UpdateServerAsync(request))
                return;
        }
    }

    [RelayCommand]
    private void DismissInstall() => IsInstalling = false;

    [RelayCommand]
    private async Task StartServer(ServerItem? item)
    {
        if (item is null)
            return;
        var tracker = item.ProgressTracker;
        RunOnUI(() =>
        {
            if (tracker is not null)
            {
                tracker.IsReadyToRun = false;
                tracker.IsActive = true;
                tracker.IsIndeterminate = true;
                tracker.CurrentStep = _localizer.Get("ServerState_Starting");
            }
        });
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.StartAsync(new ServerId { Id = item.Id }, deadline: DateTime.UtcNow.AddSeconds(60));
        }
        catch (RpcException)
        {
        }
        await RefreshAsync();
    }

    [RelayCommand]
    private async Task StopServer(ServerItem? item)
    {
        if (item is null)
            return;
        var tracker = item.ProgressTracker;
        RunOnUI(() =>
        {
            if (tracker is not null)
            {
                tracker.IsReadyToRun = false;
                tracker.IsActive = true;
                tracker.IsIndeterminate = true;
                tracker.CurrentStep = _localizer.Get("ServerState_Stopping");
            }
        });
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.StopAsync(new ServerId { Id = item.Id }, deadline: DateTime.UtcNow.AddSeconds(60));
        }
        catch (RpcException)
        {
        }
        await RefreshAsync();
    }

    [RelayCommand]
    private async Task DeleteServer(ServerItem? item)
    {
        if (item is null)
            return;
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.DeleteAsync(new ServerId { Id = item.Id }, deadline: DateTime.UtcNow.AddSeconds(60));
        }
        catch (RpcException)
        {
        }
        await RefreshAsync();
    }

    private void ApplyInstallProgress(InstallProgress progress)
    {
        if (progress.Step is { Key.Length: > 0 } step)
            InstallStep = _localizer.Get(step.Key);

        if (progress.Fraction >= 0)
        {
            InstallIsIndeterminate = false;
            InstallFraction = progress.Fraction;
        }
        else
        {
            InstallIsIndeterminate = true;
        }

        if (!string.IsNullOrEmpty(progress.LogLine))
            AppendLog(progress.LogLine);
    }

    private void AppendLog(string line)
    {
        if (line.Length > MaxLogLineLength)
            line = line[..MaxLogLineLength] + "…";
        InstallLog.Add(line);
        while (InstallLog.Count > MaxLogLines)
            InstallLog.RemoveAt(0);
    }

    private void MergeServers(System.Collections.Generic.IEnumerable<Server> incoming)
    {
        var list = incoming.ToList();
        var existing = Servers.ToDictionary(s => s.Id);

        foreach (var proto in list)
        {
            var tracker = ProgressService.GetOrCreateTracker(proto.Id, proto.Name);
            if (proto.Status is not (ServerStatus.Installing or ServerStatus.Starting or ServerStatus.Stopping))
            {
                if (tracker.IsActive)
                    tracker.IsActive = false;
            }
            else
            {
                if (!tracker.IsActive)
                    tracker.IsActive = true;
                tracker.CurrentStep = _localizer.Get(proto.Status switch
                {
                    ServerStatus.Installing => "ServerState_Installing",
                    ServerStatus.Starting => "ServerState_Starting",
                    ServerStatus.Stopping => "ServerState_Stopping",
                    _ => "ServerStatus_Unknown"
                });
            }

            if (existing.TryGetValue(proto.Id, out var item))
            {
                item.ProgressTracker = tracker;
                item.Apply(proto);
                if (TryGetPendingUpdate(proto.Id, out var pending))
                    item.ApplyLocal(pending);
            }
            else
            {
                var newItem = new ServerItem(proto, _localizer) { ProgressTracker = tracker };
                if (TryGetPendingUpdate(proto.Id, out var pending))
                    newItem.ApplyLocal(pending);
                Servers.Add(newItem);
            }
        }

        var keep = list.Select(p => p.Id).ToHashSet();
        for (var i = Servers.Count - 1; i >= 0; i--)
        {
            if (!keep.Contains(Servers[i].Id))
                Servers.RemoveAt(i);
        }

        for (var targetIndex = 0; targetIndex < list.Count; targetIndex++)
        {
            var id = list[targetIndex].Id;
            var currentIndex = Servers.Select((server, index) => (server, index))
                .FirstOrDefault(pair => pair.server.Id == id).index;
            if (currentIndex != targetIndex && currentIndex >= 0)
                Servers.Move(currentIndex, targetIndex);
        }

        OnServerStatsChanged();
    }

    private static UpdateServerRequest BuildUpdateRequest(ServerItem item) => new()
    {
        Id = item.Id,
        Name = item.Name,
        McVersion = item.McVersion,
        Port = item.Port,
        SortOrder = item.SortOrder,
        MemoryMb = item.MemoryMb,
        CustomJavaArgs = item.CustomJavaArgs,
    };

    private static UpdateServerRequest CloneUpdateRequest(UpdateServerRequest request) => new()
    {
        Id = request.Id,
        Name = request.Name,
        McVersion = request.McVersion,
        Port = request.Port,
        SortOrder = request.SortOrder,
        MemoryMb = request.MemoryMb,
        CustomJavaArgs = request.CustomJavaArgs,
    };

    private bool TryGetPendingUpdate(string serverId, out UpdateServerRequest request)
    {
        lock (_pendingUpdatesGate)
            return _pendingUpdates.TryGetValue(serverId, out request!);
    }

    private void OnServerStatsChanged()
    {
        OnPropertyChanged(nameof(TotalServers));
        OnPropertyChanged(nameof(RunningServers));
        OnPropertyChanged(nameof(StoppedServers));
        OnPropertyChanged(nameof(BusyServers));
    }

    private void StartAutoRefresh()
    {
        _refreshTimer = _dispatcher.CreateTimer();
        _refreshTimer.Interval = TimeSpan.FromSeconds(3);
        _refreshTimer.Tick += (_, _) => _ = RefreshAsync();
        _refreshTimer.Start();
    }

    private static string MapErrorKey(RpcException ex) => ex.StatusCode switch
    {
        StatusCode.NotFound => "error.version_not_found",
        StatusCode.Unavailable => "error.jre_download_failed",
        StatusCode.Unimplemented => "error.type_unsupported",
        _ => "error.install_failed",
    };

    private void RunOnUI(Action action)
    {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
