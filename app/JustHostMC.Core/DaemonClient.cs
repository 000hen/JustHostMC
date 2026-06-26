using Grpc.Core.Interceptors;
using Grpc.Net.Client;
using McManager.Grpc;

namespace JustHostMC.Core;

/// <summary>
/// Typed gRPC client for the engine over the authenticated loopback channel.
/// All calls carry the session token via <see cref="TokenInterceptor"/>.
/// </summary>
public sealed class DaemonClient : IAsyncDisposable
{
    private readonly GrpcChannel _channel;

    public EngineService.EngineServiceClient Engine { get; }
    public ServerService.ServerServiceClient Servers { get; }
    public ConsoleService.ConsoleServiceClient Console { get; }
    public BackupService.BackupServiceClient Backups { get; }
    public SettingsService.SettingsServiceClient Settings { get; }
    public PlayerService.PlayerServiceClient Players { get; }
    public MetricsService.MetricsServiceClient Metrics { get; }
    public ModService.ModServiceClient Mods { get; }

    public DaemonClient(EngineConnection connection)
        : this(connection.Port, connection.Token)
    {
    }

    public DaemonClient(int port, string token)
    {
        // The engine serves gRPC over HTTP/2 cleartext (h2c) on loopback; enable
        // unencrypted HTTP/2 for the underlying HttpClient.
        AppContext.SetSwitch("System.Net.Http.SocketsHttpHandler.Http2UnencryptedSupport", true);

        _channel = GrpcChannel.ForAddress($"http://127.0.0.1:{port}");
        var invoker = _channel.Intercept(new TokenInterceptor(token));
        Engine = new EngineService.EngineServiceClient(invoker);
        Servers = new ServerService.ServerServiceClient(invoker);
        Console = new ConsoleService.ConsoleServiceClient(invoker);
        Backups = new BackupService.BackupServiceClient(invoker);
        Settings = new SettingsService.SettingsServiceClient(invoker);
        Players = new PlayerService.PlayerServiceClient(invoker);
        Metrics = new MetricsService.MetricsServiceClient(invoker);
        Mods = new ModService.ModServiceClient(invoker);
    }

    public async ValueTask DisposeAsync() => await _channel.ShutdownAsync().ConfigureAwait(false);
}
