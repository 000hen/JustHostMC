using McManager.Grpc;

namespace JustHostMC.Core;

/// <summary>
/// Owns one server-change synchronization loop and replaces it atomically when
/// an explicit reconciliation is requested.
/// </summary>
public sealed class ServerChangeSession(
    ServerChangeSynchronizer synchronizer,
    IServerChangeSource source,
    Action<IReadOnlyList<Server>> reconcile,
    Action<ServerChangeEvent> apply) : IAsyncDisposable {
    private readonly SemaphoreSlim _gate = new(1, 1);
    private CancellationTokenSource? _cancellation;
    private Task? _run;
    private bool _disposed;

    public async Task StartAsync() {
        await _gate.WaitAsync().ConfigureAwait(false);
        try {
            ObjectDisposedException.ThrowIf(_disposed, this);
            if (_run is not null)
                return;
            StartLocked();
        } finally {
            _gate.Release();
        }
    }

    public async Task RestartAsync() {
        await _gate.WaitAsync().ConfigureAwait(false);
        try {
            ObjectDisposedException.ThrowIf(_disposed, this);
            await StopLockedAsync().ConfigureAwait(false);
            StartLocked();
        } finally {
            _gate.Release();
        }
    }

    public async ValueTask DisposeAsync() {
        await _gate.WaitAsync().ConfigureAwait(false);
        try {
            if (_disposed)
                return;
            _disposed = true;
            await StopLockedAsync().ConfigureAwait(false);
        } finally {
            _gate.Release();
        }
    }

    private void StartLocked() {
        _cancellation = new CancellationTokenSource();
        _run = synchronizer.RunAsync(source, reconcile, apply,
                                     _cancellation.Token);
    }

    private async Task StopLockedAsync() {
        if (_cancellation is null)
            return;
        await _cancellation.CancelAsync().ConfigureAwait(false);
        if (_run is not null)
            await _run.ConfigureAwait(false);
        _cancellation.Dispose();
        _cancellation = null;
        _run          = null;
    }
}
