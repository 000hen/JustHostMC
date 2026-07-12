using McManager.Grpc;

namespace JustHostMC.Core;

/// <summary>
/// Tracks optimistic server edits until the daemon returns its normalized
/// authoritative snapshot.
/// </summary>
public sealed class PendingServerUpdates {
    private readonly object _gate = new();
    private readonly Dictionary<string, UpdateServerRequest> _updates = [];

    public void Begin(UpdateServerRequest request) {
        lock (_gate)
            _updates[request.Id] = request.Clone();
    }

    public bool TryGet(string serverId, out UpdateServerRequest request) {
        lock (_gate) {
            if (_updates.TryGetValue(serverId, out var pending)) {
                request = pending.Clone();
                return true;
            }
        }
        request = null!;
        return false;
    }

    public void Complete(string serverId) {
        lock (_gate)
            _updates.Remove(serverId);
    }

    public void Cancel(string serverId) {
        lock (_gate)
            _updates.Remove(serverId);
    }
}
