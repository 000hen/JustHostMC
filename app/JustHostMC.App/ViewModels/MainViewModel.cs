using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Grpc.Core;
using JustHostMC.App.Collections;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.Core;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>
/// Backs the main window: connects to the engine, lists/creates/starts/stops
/// servers, and surfaces install progress. All user-visible text is localized
/// and every gRPC stream/callback update is marshaled to the UI thread (PROMPT
/// §10.1).
/// </summary>
public partial class MainViewModel : ObservableObject, IAsyncDisposable {
    private const int MaxLogLines      = 2000;
    private const int MaxLogLineLength = 2000;

    private readonly ILocalizer _localizer;
    private readonly DispatcherQueue _dispatcher;
    private readonly BackgroundTaskService _backgroundTasks;
    private readonly PendingServerUpdates _pendingUpdates    = new();
    private readonly ServerChangeSynchronizer _serverChanges = new();
    private readonly ServerListState _serverState            = new();
    private ServerChangeSession? _serverChangesSession;

    public ObservableCollection<ServerItem> Servers { get; } = new();
    public ObservableCollection<object> NavigationItems {
        get;
    } = new() {
        NavigationDestination.Home,
    };
    public ObservableCollection<object> FooterNavigationItems {
        get;
    } = new() {
        NavigationDestination.AddServer,
        NavigationDestination.Scripts,
        NavigationDestination.Settings,
    };
    public BoundedObservableCollection<string> InstallLog {
        get;
    } = new(MaxLogLines);
    public ServerProgressService ProgressService { get; }

    /// <summary>Cached provider list, shared with ServerItems for friendly
    /// names + capabilities.</summary>
    public ProviderCatalog ProviderCatalog { get; }

    public int TotalServers => Servers.Count;
    public int RunningServers =>
        Servers.Count(s => s.Status == ServerStatus.Running);
    public int StoppedServers => Servers.Count(
        s => s.Status is ServerStatus.Stopped or ServerStatus.Crashed);
    public int BusyServers =>
        Servers.Count(s => s.Status is ServerStatus.Installing or
                               ServerStatus.Starting or ServerStatus.Stopping);
    [ObservableProperty]
    public partial string EngineStatus { get; private set; }

    [ObservableProperty]
    public partial bool IsInstalling {
        get; private set;
    }

    [ObservableProperty]
    public partial bool InstallFailed {
        get; private set;
    }

    [ObservableProperty]
    public partial string InstallStep {
        get; private set;
    } = "";

    [ObservableProperty]
    public partial double InstallFraction {
        get; private set;
    }

    [ObservableProperty]
    public partial bool InstallIsIndeterminate {
        get; private set;
    } = true;

    public MainViewModel(ILocalizer localizer, DispatcherQueue dispatcher,
                         BackgroundTaskService backgroundTasks) {
        _localizer       = localizer;
        _dispatcher      = dispatcher;
        _backgroundTasks = backgroundTasks;
        ProgressService  = new ServerProgressService(_dispatcher);
        ProviderCatalog  = new ProviderCatalog(FetchProvidersAsync);
        EngineStatus     = _localizer.Get("EngineStatus_Connecting");
    }

    /// <summary>Waits for the engine, probes Health, and starts the server
    /// change stream.</summary>
    public async Task ConnectAsync() {
        RunOnUI(() => EngineStatus = _localizer.Get("EngineStatus_Connecting"));
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Engine.HealthAsync(
                new Empty(), deadline: DateTime.UtcNow.AddSeconds(10));
            RunOnUI(() => EngineStatus =
                        _localizer.Get("EngineStatus_Connected"));
            // Warm the provider catalog so server-type names + capabilities
            // resolve (non-fatal: ServerItem falls back to its id-based name
            // until loaded).
            _ = GetProvidersAsync().ContinueWith(
                static t => _ = t.Exception,
                TaskContinuationOptions.OnlyOnFaulted);
            _serverChangesSession ??= CreateServerChangeSession(daemon);
            await _serverChangesSession.StartAsync();
        } catch (Exception) {
            RunOnUI(() => EngineStatus = _localizer.Get("EngineStatus_Failed"));
        }
    }

    /// <summary>Fetches available versions for a provider (for the create
    /// wizard).</summary>
    public async Task<string[]> GetVersionsAsync(string providerId) {
        var daemon = await App.Current.DaemonReady;
        var list   = await daemon.Engine.ListVersionsAsync(
            new VersionQuery { ProviderId = providerId },
            deadline: DateTime.UtcNow.AddSeconds(30));
        return list.Versions.ToArray();
    }

    /// <summary>Fetches the installed provider scripts (built-in +
    /// user-imported), cached.</summary>
    public Task<IReadOnlyList<ProviderInfo>> GetProvidersAsync() =>
        ProviderCatalog.GetAllAsync();

    private async Task<IReadOnlyList<ProviderInfo>> FetchProvidersAsync() {
        var daemon = await App.Current.DaemonReady;
        var list   = await daemon.Providers.ListAsync(
            new Empty(), deadline: DateTime.UtcNow.AddSeconds(30));
        return list.Providers;
    }

    /// <summary>
    /// Suggests a unique default server name (e.g. "My Server", then "My Server
    /// (1)") for when the user creates a server without typing one.
    /// </summary>
    public string SuggestDefaultServerName() {
        var baseName          = _localizer.Get("CreateServer_DefaultName");
        bool Taken(string n) => Servers.Any(
            s => string.Equals(s.Name, n, StringComparison.OrdinalIgnoreCase));
        if (!Taken(baseName))
            return baseName;
        for (var i = 1;; i++) {
            var candidate = $"{baseName} ({i})";
            if (!Taken(candidate))
                return candidate;
        }
    }

    [RelayCommand]
    private async Task Refresh() {
        var daemon              = await App.Current.DaemonReady;
        _serverChangesSession ??= CreateServerChangeSession(daemon);
        await _serverChangesSession.RestartAsync();
    }

    /// <summary>Creates a server, streaming localized install progress + raw
    /// log.</summary>
    public async Task InstallServerAsync(CreateServerRequest request) {
        using var backgroundTask = _backgroundTasks.Begin("server-install");
        var tracker = ProgressService.GetOrCreateTracker(null, request.Name);
        RunOnUI(() => {
            tracker.InstallLog.Clear();
            tracker.HasFailed        = false;
            tracker.IsInstalling     = true;
            tracker.IsActive         = true;
            tracker.IsReadyToRun     = false;
            tracker.IsIndeterminate  = true;
            tracker.ProgressFraction = 0;
            tracker.CurrentStep = _localizer.Get("install_progress_preparing");

            InstallLog.Clear();
            InstallFailed          = false;
            IsInstalling           = true;
            InstallIsIndeterminate = true;
            InstallFraction        = 0;
            InstallStep = _localizer.Get("install_progress_preparing");
        });

        var progressBuffer = new InstallProgressBuffer(
            _dispatcher, (step, fraction, logLines) => ApplyInstallProgress(
                             step, fraction, logLines, tracker));

        try {
            var daemon     = await App.Current.DaemonReady;
            using var call = daemon.Servers.Create(request);
            await foreach (var progress in call.ResponseStream.ReadAllAsync()
                               .ConfigureAwait(false)) {
                progressBuffer.Post(progress);
            }
            await progressBuffer.FlushAsync();
            RunOnUI(() => {
                IsInstalling         = false;
                tracker.IsInstalling = false;
                tracker.IsActive     = false;
                tracker.IsReadyToRun = true;
                tracker.CurrentStep  = _localizer.Get("install_progress_done") +
                                       " " +
                                       _localizer.Get("install_ready_to_run");
            });
        } catch (RpcException ex) {
            await progressBuffer.FlushAsync();
            var key    = MapErrorKey(ex);
            var detail = ex.Status.Detail;
            RunOnUI(() => {
                IsInstalling           = false;
                InstallFailed          = true;
                InstallIsIndeterminate = false;
                InstallStep = string.IsNullOrEmpty(detail)
                                  ? _localizer.Get(key)
                                  : $"{_localizer.Get(key)}: {detail}";

                tracker.HasFailed       = true;
                tracker.IsInstalling    = false;
                tracker.IsIndeterminate = false;
                tracker.IsActive        = false;
                tracker.CurrentStep     = InstallStep;
            });
        }
    }

    public async Task<bool> UpdateServerAsync(UpdateServerRequest request) {
        var item       = Servers.FirstOrDefault(s => s.Id == request.Id);
        var rollback   = item is null ? null : BuildUpdateRequest(item);
        var optimistic = request.Clone();
        _pendingUpdates.Begin(optimistic);
        if (item is not null)
            RunOnUI(() => item.ApplyLocal(optimistic));

        try {
            var daemon  = await App.Current.DaemonReady;
            var updated = await daemon.Servers.UpdateAsync(
                optimistic, deadline: DateTime.UtcNow.AddMinutes(3));
            _pendingUpdates.Complete(updated.Id);
            RunOnUI(() => {
                if (_serverState.TryGet(updated.Id, out var authoritative))
                    ApplyServerChange(new ServerChangeEvent {
                        Upsert = authoritative,
                    });
                else if (item is null)
                    ApplyServerChange(new ServerChangeEvent {
                        Upsert = updated,
                    });
            });
            return true;
        } catch (RpcException) {
            _pendingUpdates.Cancel(optimistic.Id);
            if (item is not null && rollback is not null)
                RunOnUI(() => item.ApplyLocal(rollback));
            return false;
        }
    }

    public Task<bool> RenameServerAsync(ServerItem item, string name) {
        name = name.Trim();
        if (name.Length == 0)
            return Task.FromResult(false);

        var request  = BuildUpdateRequest(item);
        request.Name = name;
        return UpdateServerAsync(request);
    }

    public async Task MoveServerAsync(ServerItem item, int offset) {
        var ordered = Servers.ToList();
        var index   = ordered.FindIndex(s => s.Id == item.Id);
        var target  = index + offset;
        if (index < 0 || target < 0 || target >= ordered.Count)
            return;

        ordered.RemoveAt(index);
        ordered.Insert(target, item);

        for (var i = 0; i < ordered.Count; i++) {
            var request       = BuildUpdateRequest(ordered[i]);
            request.SortOrder = i;
            if (!await UpdateServerAsync(request))
                return;
        }
    }

    [RelayCommand]
    private void DismissInstall() => IsInstalling = false;

    [RelayCommand]
    private async Task StartServer(ServerItem? item) {
        if (item is null)
            return;
        using var backgroundTask = _backgroundTasks.Begin("server-start");
        var tracker              = item.ProgressTracker;
        RunOnUI(() => {
            if (tracker is not null) {
                tracker.IsReadyToRun    = false;
                tracker.IsActive        = true;
                tracker.IsIndeterminate = true;
                tracker.CurrentStep = _localizer.Get("ServerState_Starting");
            }
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.StartAsync(
                new ServerId { Id = item.Id },
                deadline: DateTime.UtcNow.AddSeconds(60));
        } catch (RpcException) {
        }
    }

    [RelayCommand]
    private async Task StopServer(ServerItem? item) {
        if (item is null)
            return;
        using var backgroundTask = _backgroundTasks.Begin("server-stop");
        var tracker              = item.ProgressTracker;
        RunOnUI(() => {
            if (tracker is not null) {
                tracker.IsReadyToRun    = false;
                tracker.IsActive        = true;
                tracker.IsIndeterminate = true;
                tracker.CurrentStep = _localizer.Get("ServerState_Stopping");
            }
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.StopAsync(
                new ServerId { Id = item.Id },
                deadline: DateTime.UtcNow.AddSeconds(60));
        } catch (RpcException) {
        }
    }

    [RelayCommand]
    private async Task DeleteServer(ServerItem? item) {
        if (item is null)
            return;
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Servers.DeleteAsync(
                new ServerId { Id = item.Id },
                deadline: DateTime.UtcNow.AddSeconds(60));
        } catch (RpcException) {
        }
    }

    private void ApplyInstallProgress(LocalizedMessage? step, double fraction,
                                      IReadOnlyList<string> logLines,
                                      ServerProgressTracker tracker) {
        if (step is { Key.Length : > 0 }) {
            InstallStep         = _localizer.Get(step.Key);
            tracker.CurrentStep = InstallStep;
        }

        if (fraction >= 0) {
            InstallIsIndeterminate   = false;
            InstallFraction          = fraction;
            tracker.IsIndeterminate  = false;
            tracker.ProgressFraction = fraction;
        } else {
            InstallIsIndeterminate  = true;
            tracker.IsIndeterminate = true;
        }

        var normalized =
            logLines
                .Select(line => line.Length > MaxLogLineLength
                                    ? line[..MaxLogLineLength] + "…"
                                    : line)
                .ToArray();
        InstallLog.AddRange(normalized);
        tracker.AppendLogs(normalized);
    }

    private void MergeServers(
        System.Collections.Generic.IEnumerable<Server> incoming) {
        var list = incoming.ToList();
        _serverState.Reconcile(list);
        _backgroundTasks.SynchronizeServers(list);

        foreach (var proto in list) UpsertServer(proto);

        var keep = list.Select(p => p.Id).ToHashSet();
        for (var i = Servers.Count - 1; i >= 0; i--) {
            if (!keep.Contains(Servers[i].Id))
                RemoveServer(Servers[i].Id);
        }

        ReorderServers(_serverState.Servers.Select(server => server.Id));
        OnServerStatsChanged();
    }

    private void ApplyServerChange(ServerChangeEvent change) {
        _serverState.Apply(change);
        switch (change.ChangeCase) {
            case ServerChangeEvent.ChangeOneofCase.Upsert:
                UpsertServer(change.Upsert);
                ReorderServers(
                    _serverState.Servers.Select(server => server.Id));
                break;
            case ServerChangeEvent.ChangeOneofCase.Deleted:
                RemoveServer(change.Deleted.Id);
                break;
        }
        OnServerStatsChanged();
    }

    private void UpsertServer(Server proto) {
        var tracker = ProgressService.GetOrCreateTracker(proto.Id, proto.Name);
        if (proto.Status is not(ServerStatus.Installing or ServerStatus
                                    .Starting or ServerStatus.Stopping)) {
            if (tracker.IsActive)
                tracker.IsActive = false;
        } else {
            if (!tracker.IsActive)
                tracker.IsActive = true;
            tracker.CurrentStep = _localizer.Get(proto.Status switch {
                ServerStatus.Installing => "ServerState_Installing",
                ServerStatus.Starting   => "ServerState_Starting",
                ServerStatus.Stopping   => "ServerState_Stopping",
                _                       => "ServerStatus_Unknown"
            });
        }

        var item = Servers.FirstOrDefault(server => server.Id == proto.Id);
        if (item is not null) {
            item.ProgressTracker = tracker;
            item.Apply(proto);
            if (TryGetPendingUpdate(proto.Id, out var pending))
                item.ApplyLocal(pending);
            return;
        }

        var newItem = new ServerItem(proto, _localizer, ProviderCatalog,
                                     _dispatcher) { ProgressTracker = tracker };
        if (TryGetPendingUpdate(proto.Id, out var newPending))
            newItem.ApplyLocal(newPending);
        Servers.Add(newItem);
        NavigationItems.Add(newItem);
    }

    private void RemoveServer(string serverId) {
        var item = Servers.FirstOrDefault(server => server.Id == serverId);
        if (item is null)
            return;
        Servers.Remove(item);
        NavigationItems.Remove(item);
    }

    private void ReorderServers(IEnumerable<string> orderedIds) {
        var targetIndex = 0;
        foreach (var id in orderedIds) {
            var currentIndex = -1;
            for (var i = 0; i < Servers.Count; i++) {
                if (Servers[i].Id != id)
                    continue;
                currentIndex = i;
                break;
            }
            if (currentIndex < 0)
                continue;
            if (currentIndex != targetIndex) {
                Servers.Move(currentIndex, targetIndex);
                NavigationItems.Move(currentIndex + 1, targetIndex + 1);
            }
            targetIndex++;
        }
    }

    private static UpdateServerRequest BuildUpdateRequest(ServerItem item) =>
        new() {
            Id             = item.Id,
            Name           = item.Name,
            McVersion      = item.McVersion,
            Port           = item.Port,
            SortOrder      = item.SortOrder,
            MemoryMb       = item.MemoryMb,
            CustomJavaArgs = item.CustomJavaArgs,
        };

    private bool TryGetPendingUpdate(string serverId,
                                     out UpdateServerRequest request) =>
        _pendingUpdates.TryGet(serverId, out request);

    private void OnServerStatsChanged() {
        OnPropertyChanged(nameof(TotalServers));
        OnPropertyChanged(nameof(RunningServers));
        OnPropertyChanged(nameof(StoppedServers));
        OnPropertyChanged(nameof(BusyServers));
    }

    private ServerChangeSession CreateServerChangeSession(
        DaemonClient daemon) =>
        new(_serverChanges, new GrpcServerChangeSource(daemon.Servers),
            servers => RunOnUI(() => MergeServers(servers)),
            change  => RunOnUI(() => ApplyServerChange(change)));

    public async ValueTask DisposeAsync() {
        if (_serverChangesSession is not null)
            await _serverChangesSession.DisposeAsync();
    }

    private static string MapErrorKey(RpcException ex) => ex.StatusCode switch {
        StatusCode.NotFound      => "error.version_not_found",
        StatusCode.Unavailable   => "error.jre_download_failed",
        StatusCode.Unimplemented => "error.type_unsupported",
        _                        => "error.install_failed",
    };

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }

    private sealed class InstallProgressBuffer(
        DispatcherQueue dispatcher,
        Action<LocalizedMessage?, double, IReadOnlyList<string>> apply) {
        private readonly object _gate           = new();
        private readonly List<string> _logLines = new();
        private LocalizedMessage? _step;
        private double _fraction = -1;
        private bool _hasProgress;
        private bool _scheduled;

        public void Post(InstallProgress progress) {
            lock (_gate) {
                if (progress.Step is { Key.Length : > 0 })
                    _step = progress.Step;
                _fraction    = progress.Fraction;
                _hasProgress = true;
                if (!string.IsNullOrEmpty(progress.LogLine))
                    _logLines.Add(progress.LogLine);

                if (_scheduled)
                    return;

                _scheduled = true;
                if (!dispatcher.TryEnqueue(DispatcherQueuePriority.Low, Drain))
                    _scheduled = false;
            }
        }

        public Task FlushAsync() {
            if (dispatcher.HasThreadAccess) {
                Drain();
                return Task.CompletedTask;
            }

            var completion = new TaskCompletionSource(
                TaskCreationOptions.RunContinuationsAsynchronously);
            if (!dispatcher.TryEnqueue(DispatcherQueuePriority.Low, () => {
                    Drain();
                    completion.SetResult();
                })) {
                completion.SetResult();
            }
            return completion.Task;
        }

        private void Drain() {
            LocalizedMessage? step;
            double fraction;
            string[] lines;
            bool hasProgress;
            lock (_gate) {
                hasProgress  = _hasProgress;
                step         = _step;
                fraction     = _fraction;
                lines        = _logLines.ToArray();
                _step        = null;
                _fraction    = -1;
                _hasProgress = false;
                _logLines.Clear();
                _scheduled = false;
            }

            if (hasProgress)
                apply(step, fraction, lines);
        }
    }
}
