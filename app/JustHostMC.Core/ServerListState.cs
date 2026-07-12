using McManager.Grpc;

namespace JustHostMC.Core;

/// <summary>
/// Maintains an ordered, UI-independent projection of server list snapshots.
/// </summary>
public sealed class ServerListState {
    private readonly List<Server> _servers = [];

    public IReadOnlyList<Server> Servers => _servers;

    public void Reconcile(IEnumerable<Server> servers) {
        _servers.Clear();
        _servers.AddRange(servers.Select(server => server.Clone()));
        Sort();
    }

    public void Apply(ServerChangeEvent change) {
        switch (change.ChangeCase) {
        case ServerChangeEvent.ChangeOneofCase.Upsert:
            var index = _servers.FindIndex(server =>
                                              server.Id == change.Upsert.Id);
            var snapshot = change.Upsert.Clone();
            if (index >= 0)
                _servers[index] = snapshot;
            else
                _servers.Add(snapshot);
            Sort();
            break;
        case ServerChangeEvent.ChangeOneofCase.Deleted:
            _servers.RemoveAll(server => server.Id == change.Deleted.Id);
            break;
        }
    }

    public Server LatestOr(Server fallback) {
        var latest = _servers.FirstOrDefault(server =>
                                                 server.Id == fallback.Id);
        return (latest ?? fallback).Clone();
    }

    private void Sort() => _servers.Sort(static (left, right) => {
        var order = left.SortOrder.CompareTo(right.SortOrder);
        if (order != 0)
            return order;
        order = string.Compare(left.Name, right.Name,
                               StringComparison.Ordinal);
        return order != 0
                   ? order
                   : string.Compare(left.Id, right.Id,
                                    StringComparison.Ordinal);
    });
}
