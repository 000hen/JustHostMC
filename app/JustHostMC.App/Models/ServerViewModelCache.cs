using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.Models;

/// <summary>
/// Holds the four per-server view models so they survive page navigation.
/// gRPC streams stay open across visits, making re-visits instant.
/// Live stream attach is parallelized and idempotent; heavier tab data is
/// warmed separately so page navigation can render immediately.
/// </summary>
public sealed class ServerViewModelCache : IAsyncDisposable {
    public ConsoleViewModel Console { get; }
    public PlayersViewModel Players { get; }
    public MetricsViewModel Metrics { get; }
    public ModsViewModel Mods { get; }
    public ServerConfigViewModel Config { get; }

    private bool _attached;
    private Task? _preloadTask;

    public ServerViewModelCache(string serverId, string serverName,
                                DispatcherQueue dispatcher,
                                ILocalizer localizer) {
        Console = new ConsoleViewModel(serverId, serverName, dispatcher);
        Players = new PlayersViewModel(serverId, dispatcher);
        Metrics = new MetricsViewModel(serverId, dispatcher);
        Mods    = new ModsViewModel(serverId, dispatcher, localizer);
        Config  = new ServerConfigViewModel(serverId, dispatcher, localizer);
    }

    /// <summary>Attaches live gRPC streams in parallel. No-op on repeat
    /// calls.</summary>
    public async Task AttachAsync() {
        if (_attached)
            return;
        _attached = true;
        await Task.WhenAll(Console.AttachAsync(), Players.AttachAsync(),
                           Metrics.AttachAsync());
    }

    /// <summary>Warms heavier tab data after navigation has rendered.</summary>
    public Task PreloadAsync() {
        if (_preloadTask is { IsCompleted : false })
            return _preloadTask;

        _preloadTask = Mods.EnsureLoadedAsync();
        return _preloadTask;
    }

    /// <summary>Tears down all streams in parallel.</summary>
    public async ValueTask DisposeAsync() {
        await Task.WhenAll(Console.DisposeAsync().AsTask(),
                           Players.DisposeAsync().AsTask(),
                           Metrics.DisposeAsync().AsTask());
    }
}
