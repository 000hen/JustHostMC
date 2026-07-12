using System.Runtime.CompilerServices;
using System.Threading.Channels;
using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

public class ServerChangeSessionTests {
    [Fact]
    public async Task
    RestartAsync_CancelsOldStreamAndRunsFreshReadyListHandshake() {
        var source = new RestartableSource();
        var synchronizer =
            new ServerChangeSynchronizer((_, _) => Task.CompletedTask);
        var session = new ServerChangeSession(synchronizer, source,
                                              _ => {},
                                              _ => {});

        await session.StartAsync();
        var first =
            await source.NextWatch.Reader.ReadAsync().AsTask().WaitAsync(
                TimeSpan.FromSeconds(1));
        await first.Writer.WriteAsync(new ServerChangeEvent {
            Ready = new Empty(),
        });
        await source.WaitForListsAsync(1);

        await session.RestartAsync();
        var second =
            await source.NextWatch.Reader.ReadAsync().AsTask().WaitAsync(
                TimeSpan.FromSeconds(1));
        await second.Writer.WriteAsync(new ServerChangeEvent {
            Ready = new Empty(),
        });
        await source.WaitForListsAsync(2);

        Assert.Equal(2, source.WatchCount);
        Assert.Equal(2, source.ListCount);
        await session.DisposeAsync();
        await session.DisposeAsync();
    }

    private sealed class RestartableSource : IServerChangeSource {
        private readonly object _gate             = new();
        private TaskCompletionSource _listChanged = NewSignal();

        public Channel<Channel<ServerChangeEvent>> NextWatch {
            get;
        } = Channel.CreateUnbounded<Channel<ServerChangeEvent>>();
        public int WatchCount { get; private set; }
        public int ListCount { get; private set; }

        public async IAsyncEnumerable<ServerChangeEvent> WatchAsync(
            [EnumeratorCancellation] CancellationToken cancellationToken) {
            WatchCount++;
            var events = Channel.CreateUnbounded<ServerChangeEvent>();
            await NextWatch.Writer.WriteAsync(events, cancellationToken);
            await foreach (var change in events.Reader.ReadAllAsync(
                               cancellationToken)) yield return change;
        }

        public Task<IReadOnlyList<Server>> ListAsync(
            CancellationToken cancellationToken) {
            lock (_gate) {
                ListCount++;
                _listChanged.TrySetResult();
                _listChanged = NewSignal();
            }
            IReadOnlyList<Server> servers = [];
            return Task.FromResult(servers);
        }

        public async Task WaitForListsAsync(int count) {
            while (true) {
                Task signal;
                lock (_gate) {
                    if (ListCount >= count)
                        return;
                    signal = _listChanged.Task;
                }
                await signal.WaitAsync(TimeSpan.FromSeconds(1));
            }
        }

        private static TaskCompletionSource NewSignal() =>
            new(TaskCreationOptions.RunContinuationsAsynchronously);
    }
}
