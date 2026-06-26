using System;
using System.Threading.Tasks;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.Models;

/// <summary>
/// Holds the four per-server view models so they survive page navigation.
/// gRPC streams stay open across visits, making re-visits instant.
/// Attach is parallelized (Task.WhenAll) and idempotent.
/// </summary>
public sealed class ServerViewModelCache : IAsyncDisposable
{
    public ConsoleViewModel Console { get; }
    public PlayersViewModel Players { get; }
    public MetricsViewModel Metrics { get; }
    public ModsViewModel Mods { get; }

    private bool _attached;

    public ServerViewModelCache(string serverId, string serverName,
        DispatcherQueue dispatcher, ILocalizer localizer)
    {
        Console = new ConsoleViewModel(serverId, serverName, dispatcher);
        Players = new PlayersViewModel(serverId, dispatcher);
        Metrics = new MetricsViewModel(serverId, dispatcher);
        Mods = new ModsViewModel(serverId, dispatcher, localizer);
    }

    /// <summary>Attaches all four gRPC streams in parallel. No-op on repeat calls.</summary>
    public async Task AttachAsync()
    {
        if (_attached) return;
        _attached = true;
        await Task.WhenAll(
            Console.AttachAsync(),
            Players.AttachAsync(),
            Metrics.AttachAsync(),
            Mods.RefreshAsync()
        );
    }

    /// <summary>Tears down all streams in parallel.</summary>
    public async ValueTask DisposeAsync()
    {
        await Task.WhenAll(
            Console.DisposeAsync().AsTask(),
            Players.DisposeAsync().AsTask(),
            Metrics.DisposeAsync().AsTask()
        );
    }
}
