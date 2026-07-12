using System.Runtime.CompilerServices;
using Grpc.Core;
using McManager.Grpc;

namespace JustHostMC.Core;

public interface IServerChangeSource {
    IAsyncEnumerable<ServerChangeEvent> WatchAsync(
        CancellationToken cancellationToken);
    Task<IReadOnlyList<Server>> ListAsync(CancellationToken cancellationToken);
}

/// <summary>Adapts the generated gRPC client to the synchronization
/// loop.</summary>
public sealed class GrpcServerChangeSource(
    ServerService.ServerServiceClient client)
    : IServerChangeSource {
    public async IAsyncEnumerable<ServerChangeEvent> WatchAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken) {
        using var call = client.WatchChanges(
            new Empty(), cancellationToken: cancellationToken);
        await foreach (var change in call.ResponseStream
                           .ReadAllAsync(cancellationToken)
                           .ConfigureAwait(false)) yield return change;
    }

    public async Task<IReadOnlyList<Server>> ListAsync(
        CancellationToken cancellationToken) {
        var list = await client
                       .ListAsync(new Empty(),
                                  deadline: DateTime.UtcNow.AddSeconds(10),
                                  cancellationToken: cancellationToken)
                       .ConfigureAwait(false);
        return list.Servers;
    }
}
