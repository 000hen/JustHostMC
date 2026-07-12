using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

public class ServerListStateTests {
    [Fact]
    public void Apply_UpsertsDeletesAndSortsSingleServers() {
        var state = new ServerListState();
        state.Reconcile([
            new Server { Id = "b", Name = "B", SortOrder = 1 },
            new Server { Id = "a", Name = "A", SortOrder = 0 },
        ]);

        state.Apply(new ServerChangeEvent {
            Upsert = new Server { Id = "b", Name = "B2", SortOrder = -1 },
        });

        Assert.Equal(["b", "a"], state.Servers.Select(server => server.Id));
        Assert.Equal("B2", state.Servers[0].Name);

        state.Apply(new ServerChangeEvent {
            Deleted = new ServerId { Id = "a" },
        });

        Assert.Single(state.Servers);
        Assert.Equal("b", state.Servers[0].Id);
    }

    [Fact]
    public void Apply_UnknownDeleteIsNoOp() {
        var state = new ServerListState();
        state.Reconcile([new Server { Id = "one", Name = "One" }]);

        state.Apply(new ServerChangeEvent {
            Deleted = new ServerId { Id = "missing" },
        });

        Assert.Equal("one", Assert.Single(state.Servers).Id);
    }

    [Fact]
    public void Reconcile_ReplacesStaleStateAndClonesInput() {
        var state = new ServerListState();
        state.Reconcile([new Server { Id = "stale", Name = "Stale" }]);
        var current = new Server { Id = "current", Name = "Current" };

        state.Reconcile([current]);
        current.Name = "Mutated";

        var server = Assert.Single(state.Servers);
        Assert.Equal("current", server.Id);
        Assert.Equal("Current", server.Name);
    }
}
