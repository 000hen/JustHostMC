using System.Runtime.CompilerServices;
using System.Threading.Channels;
using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

public class ServerChangeSynchronizerTests {
    [Fact]
    public async Task RunAsync_WaitsForReadyThenListsBeforeApplyingChanges() {
        var source             = new ChannelServerChangeSource([
            new Server { Id = "initial", Name = "Initial" },
        ]);
        using var cancellation = new CancellationTokenSource();
        var reconciled = new TaskCompletionSource<IReadOnlyList<Server>>(
            TaskCreationOptions.RunContinuationsAsynchronously);
        var applied = new TaskCompletionSource<ServerChangeEvent>(
            TaskCreationOptions.RunContinuationsAsynchronously);
        var synchronizer =
            new ServerChangeSynchronizer((_, _) => Task.CompletedTask);

        var run = synchronizer.RunAsync(
            source, servers => reconciled.TrySetResult(servers),
            change => applied.TrySetResult(change), cancellation.Token);
        await source.WatchStarted.Task.WaitAsync(TimeSpan.FromSeconds(1));
        Assert.Equal(0, source.ListCount);

        await source.Events.Writer.WriteAsync(new ServerChangeEvent {
            Ready = new Empty(),
        });
        var initial = await reconciled.Task.WaitAsync(TimeSpan.FromSeconds(1));
        Assert.Equal("initial", Assert.Single(initial).Id);
        Assert.Equal(1, source.ListCount);

        await source.Events.Writer.WriteAsync(new ServerChangeEvent {
            Upsert = new Server { Id = "changed", Name = "Changed" },
        });
        var change = await applied.Task.WaitAsync(TimeSpan.FromSeconds(1));
        Assert.Equal("changed", change.Upsert.Id);

        cancellation.Cancel();
        await run.WaitAsync(TimeSpan.FromSeconds(1));
    }

    [Fact]
    public async Task RunAsync_ReconnectsAndRelistsAfterStreamCompletion() {
        var source             = new ReconnectingServerChangeSource();
        using var cancellation = new CancellationTokenSource();
        var reconciliations    = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        var reconcileCount = 0;
        var delayCount     = 0;
        var synchronizer   = new ServerChangeSynchronizer((_, _) => {
            Interlocked.Increment(ref delayCount);
            return Task.CompletedTask;
        });

        var run = synchronizer.RunAsync(
            source,
            _ => {
                if (Interlocked.Increment(ref reconcileCount) == 2)
                    reconciliations.TrySetResult();
            },
            _ => {}, cancellation.Token);

        await reconciliations.Task.WaitAsync(TimeSpan.FromSeconds(1));
        Assert.Equal(2, source.WatchCount);
        Assert.Equal(2, source.ListCount);
        Assert.True(delayCount >= 1);

        cancellation.Cancel();
        await run.WaitAsync(TimeSpan.FromSeconds(1));
    }

    [Fact]
    public async Task
    RunAsync_AppliesChangesQueuedWhileListIsInFlightAfterList() {
        var source             = new GatedListSource();
        using var cancellation = new CancellationTokenSource();
        var order              = new List<string>();
        var applied            = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        var synchronizer =
            new ServerChangeSynchronizer((_, _) => Task.CompletedTask);

        var run = synchronizer.RunAsync(source,
                                        _ => order.Add("list"),
                                        _ => {
                                            order.Add("event");
                                            applied.TrySetResult();
                                        },
                                        cancellation.Token);
        await source.WatchStarted.Task.WaitAsync(TimeSpan.FromSeconds(1));
        await source.Events.Writer.WriteAsync(new ServerChangeEvent {
            Ready = new Empty(),
        });
        await source.ListStarted.Task.WaitAsync(TimeSpan.FromSeconds(1));
        await source.Events.Writer.WriteAsync(new ServerChangeEvent {
            Upsert = new Server { Id = "newer" },
        });
        Assert.Empty(order);

        source.ReleaseList.TrySetResult();
        await applied.Task.WaitAsync(TimeSpan.FromSeconds(1));
        Assert.Equal(["list", "event"], order);

        cancellation.Cancel();
        await run.WaitAsync(TimeSpan.FromSeconds(1));
    }

    private sealed class ChannelServerChangeSource(
        IReadOnlyList<Server> initial)
        : IServerChangeSource {
        public Channel<ServerChangeEvent> Events {
            get;
        } = Channel.CreateUnbounded<ServerChangeEvent>();
        public TaskCompletionSource WatchStarted {
            get;
        } = new(TaskCreationOptions.RunContinuationsAsynchronously);
        public int ListCount { get; private set; }

        public async IAsyncEnumerable<ServerChangeEvent> WatchAsync(
            [EnumeratorCancellation] CancellationToken cancellationToken) {
            WatchStarted.TrySetResult();
            await foreach (var change in Events.Reader.ReadAllAsync(
                               cancellationToken)) yield return change;
        }

        public Task<IReadOnlyList<Server>> ListAsync(
            CancellationToken cancellationToken) {
            ListCount++;
            return Task.FromResult(initial);
        }
    }

    private sealed class ReconnectingServerChangeSource : IServerChangeSource {
        public int WatchCount { get; private set; }
        public int ListCount { get; private set; }

        public async IAsyncEnumerable<ServerChangeEvent> WatchAsync(
            [EnumeratorCancellation] CancellationToken cancellationToken) {
            var attempt = ++WatchCount;
            yield return new ServerChangeEvent { Ready = new Empty() };
            if (attempt == 1)
                yield break;
            await Task.Delay(Timeout.InfiniteTimeSpan, cancellationToken);
        }

        public Task<IReadOnlyList<Server>> ListAsync(
            CancellationToken cancellationToken) {
            ListCount++;
            IReadOnlyList<Server> servers = [];
            return Task.FromResult(servers);
        }
    }

    private sealed class GatedListSource : IServerChangeSource {
        public Channel<ServerChangeEvent> Events {
            get;
        } = Channel.CreateUnbounded<ServerChangeEvent>();
        public TaskCompletionSource WatchStarted {
            get;
        } = new(TaskCreationOptions.RunContinuationsAsynchronously);
        public TaskCompletionSource ListStarted {
            get;
        } = new(TaskCreationOptions.RunContinuationsAsynchronously);
        public TaskCompletionSource ReleaseList {
            get;
        } = new(TaskCreationOptions.RunContinuationsAsynchronously);

        public async IAsyncEnumerable<ServerChangeEvent> WatchAsync(
            [EnumeratorCancellation] CancellationToken cancellationToken) {
            WatchStarted.TrySetResult();
            await foreach (var change in Events.Reader.ReadAllAsync(
                               cancellationToken)) yield return change;
        }

        public async Task<IReadOnlyList<Server>> ListAsync(
            CancellationToken cancellationToken) {
            ListStarted.TrySetResult();
            await ReleaseList.Task.WaitAsync(cancellationToken);
            return [];
        }
    }
}
