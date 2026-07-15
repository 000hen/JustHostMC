using McManager.Grpc;

namespace JustHostMC.Core;

/// <summary>
/// Establishes a race-free ready/list/change sequence and reconnects after a
/// transient stream failure.
/// </summary>
public sealed class ServerChangeSynchronizer(
    Func<TimeSpan, CancellationToken, Task>? delay = null) {
    private static readonly TimeSpan InitialRetryDelay =
        TimeSpan.FromMilliseconds(250);
    private static readonly TimeSpan MaximumRetryDelay =
        TimeSpan.FromSeconds(5);
    private readonly Func<TimeSpan, CancellationToken, Task> _delay =
        delay ?? Task.Delay;

    public async Task RunAsync(IServerChangeSource source,
                               Action<IReadOnlyList<Server>> reconcile,
                               Action<ServerChangeEvent> apply,
                               CancellationToken cancellationToken) {
        var retryDelay = InitialRetryDelay;
        while (!cancellationToken.IsCancellationRequested) {
            try {
                await using var changes =
                    source.WatchAsync(cancellationToken)
                        .GetAsyncEnumerator(cancellationToken);
                if (!await changes.MoveNextAsync().ConfigureAwait(false) ||
                    changes.Current.ChangeCase is not
                        ServerChangeEvent.ChangeOneofCase.Ready)
                    throw new InvalidOperationException(
                        "server change stream did not begin with ready");

                var servers = await source.ListAsync(cancellationToken)
                                  .ConfigureAwait(false);
                reconcile(servers);
                retryDelay = InitialRetryDelay;

                while (await changes.MoveNextAsync().ConfigureAwait(false))
                    apply(changes.Current);

                throw new InvalidOperationException(
                    "server change stream completed unexpectedly");
            } catch (OperationCanceledException)
                when (cancellationToken.IsCancellationRequested) {
                return;
            } catch {
                try {
                    await _delay(retryDelay, cancellationToken)
                        .ConfigureAwait(false);
                } catch (OperationCanceledException)
                    when (cancellationToken.IsCancellationRequested) {
                    return;
                }
                retryDelay = TimeSpan.FromMilliseconds(
                    Math.Min(retryDelay.TotalMilliseconds * 2,
                             MaximumRetryDelay.TotalMilliseconds));
            }
        }
    }
}
