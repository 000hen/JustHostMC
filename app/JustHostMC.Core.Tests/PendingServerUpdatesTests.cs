using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

public class PendingServerUpdatesTests {
    [Fact]
    public void Complete_ClearsOptimisticOverlayBeforeReturningAuthoritativeEvent() {
        var pending = new PendingServerUpdates();
        pending.Begin(new UpdateServerRequest {
            Id = "one", Name = "One", Port = 0,
        });

        Assert.True(pending.TryGet("one", out var optimistic));
        Assert.Equal(0, optimistic.Port);

        var authoritative = pending.Complete(new Server {
            Id = "one", Name = "One", Port = 25565,
        });

        Assert.False(pending.TryGet("one", out _));
        Assert.Equal(ServerChangeEvent.ChangeOneofCase.Upsert,
                     authoritative.ChangeCase);
        Assert.Equal(25565, authoritative.Upsert.Port);
    }

    [Fact]
    public void Cancel_RemovesPendingUpdate() {
        var pending = new PendingServerUpdates();
        pending.Begin(new UpdateServerRequest { Id = "one" });

        pending.Cancel("one");

        Assert.False(pending.TryGet("one", out _));
    }
}
