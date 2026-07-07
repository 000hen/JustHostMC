using System;
using System.Collections.ObjectModel;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using Grpc.Core;
using JustHostMC.App.Models;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>Streams a server's online roster from the PlayerService. The roster
/// is derived engine-side from the console stream, so it reflects join/leave
/// events.</summary>
public sealed partial class PlayersViewModel : ObservableObject,
                                               IAsyncDisposable {
    private readonly string _serverId;
    private readonly DispatcherQueue _dispatcher;
    private CancellationTokenSource? _cts;

    public PlayersViewModel(string serverId, DispatcherQueue dispatcher) {
        _serverId   = serverId;
        _dispatcher = dispatcher;
    }

    public ObservableCollection<PlayerItem> Players { get; } = new();

    [ObservableProperty]
    public partial int Count {
        get; private set;
    }

    /// <summary>Opens the roster stream and applies updates on the UI
    /// thread.</summary>
    public async Task AttachAsync() {
        var daemon = await App.Current.DaemonReady;
        _cts       = new CancellationTokenSource();
        _          = ReadLoopAsync(daemon, _cts.Token);
    }

    private async Task ReadLoopAsync(JustHostMC.Core.DaemonClient daemon,
                                     CancellationToken token) {
        try {
            using var call = daemon.Players.Watch(
                new ServerId { Id = _serverId }, cancellationToken: token);
            await foreach (var list in call.ResponseStream.ReadAllAsync(token)
                               .ConfigureAwait(false)) {
                var snapshot = list;
                RunOnUI(() => Replace(snapshot));
            }
        } catch (OperationCanceledException) {
        } catch (RpcException) {
        }
    }

    private void Replace(PlayerList list) {
        Players.Clear();
        foreach (var player in list.Players)
            Players.Add(new PlayerItem(player.Name, player.Uuid));
        Count = Players.Count;
    }

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }

    public ValueTask DisposeAsync() {
        _cts?.Cancel();
        _cts?.Dispose();
        return ValueTask.CompletedTask;
    }
}
