using McManager.Grpc;

namespace JustHostMC.App.Services;

/// <summary>Central registry for work that must keep the engine alive when the
/// main window is closed or hidden.</summary>
public sealed class BackgroundTaskService {
    private const string ServerPrefix = "server:";
    private readonly object _gate = new();
    private readonly HashSet<string> _tasks = new(StringComparer.Ordinal);

    public bool HasActiveTasks {
        get {
            lock (_gate) return _tasks.Count > 0;
        }
    }

    /// <summary>Tracks a scoped operation until the returned lease is
    /// disposed.</summary>
    public IDisposable Begin(string category) {
        var key = $"operation:{category}:{Guid.NewGuid():N}";
        lock (_gate) _tasks.Add(key);
        return new Lease(this, key);
    }

    /// <summary>Replaces the persistent server-process portion of the
    /// registry from the latest engine snapshot.</summary>
    public void SynchronizeServers(IEnumerable<Server> servers) {
        lock (_gate) {
            _tasks.RemoveWhere(task => task.StartsWith(
                                   ServerPrefix, StringComparison.Ordinal));
            foreach (var server in servers) {
                if (server.Status is ServerStatus.Running or
                    ServerStatus.Installing or ServerStatus.Starting or
                    ServerStatus.Stopping)
                    _tasks.Add(ServerPrefix + server.Id);
            }
        }
    }

    private void Complete(string key) {
        lock (_gate) _tasks.Remove(key);
    }

    private sealed class Lease(BackgroundTaskService owner, string key)
        : IDisposable {
        private BackgroundTaskService? _owner = owner;

        public void Dispose() =>
            Interlocked.Exchange(ref _owner, null)?.Complete(key);
    }
}
