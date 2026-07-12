using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

public class PendingServerUpdatesTests {
    [Fact]
    public void Complete_ClearsOverlayAndKeepsNewerStreamSnapshotAuthoritative() {
        var pending = new PendingServerUpdates();
        var state = new ServerListState();
        pending.Begin(new UpdateServerRequest {
            Id = "one", Name = "One", Port = 0,
        });

        Assert.True(pending.TryGet("one", out var optimistic));
        Assert.Equal(0, optimistic.Port);

        state.Apply(new ServerChangeEvent {
            Upsert = new Server {
                Id = "one", Name = "One", Port = 25565,
                Status = ServerStatus.Crashed,
            },
        });
        pending.Complete("one");
        Assert.True(state.TryGet("one", out var authoritative));

        Assert.False(pending.TryGet("one", out _));
        Assert.Equal(25565, authoritative.Port);
        Assert.Equal(ServerStatus.Crashed, authoritative.Status);
    }

    [Fact]
    public void Complete_DoesNotResurrectServerDeletedAfterUpdate() {
        var pending = new PendingServerUpdates();
        var state = new ServerListState();
        state.Reconcile([new Server { Id = "one", Name = "One" }]);
        pending.Begin(new UpdateServerRequest { Id = "one", Name = "One" });

        state.Apply(new ServerChangeEvent {
            Deleted = new ServerId { Id = "one" },
        });
        pending.Complete("one");

        Assert.False(pending.TryGet("one", out _));
        Assert.False(state.TryGet("one", out _));
    }

    [Fact]
    public void Cancel_RemovesPendingUpdate() {
        var pending = new PendingServerUpdates();
        pending.Begin(new UpdateServerRequest { Id = "one" });

        pending.Cancel("one");

        Assert.False(pending.TryGet("one", out _));
    }
}
