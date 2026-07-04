using System.IO.Pipes;
using Grpc.Net.Client;
using McManager.Grpc;

namespace JustHostMC.Core;

/// <summary>
/// Typed gRPC client for the engine over a Windows named pipe channel.
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
    public ConfigService.ConfigServiceClient Config { get; }
    public ProviderService.ProviderServiceClient Providers { get; }
    public ScriptService.ScriptServiceClient Scripts { get; }
    public ParserService.ParserServiceClient Parsers { get; }

    public DaemonClient(EngineConnection connection)
        : this(connection.PipeName)
    {
    }

    public DaemonClient(string pipeName)
    {
        var connectionFactory = new NamedPipeConnectionFactory(pipeName);
        var socketsHandler = new SocketsHttpHandler
        {
            ConnectCallback = connectionFactory.ConnectAsync,
        };

        _channel = GrpcChannel.ForAddress("http://localhost", new GrpcChannelOptions
        {
            HttpHandler = socketsHandler,
        });

        Engine = new EngineService.EngineServiceClient(_channel);
        Servers = new ServerService.ServerServiceClient(_channel);
        Console = new ConsoleService.ConsoleServiceClient(_channel);
        Backups = new BackupService.BackupServiceClient(_channel);
        Settings = new SettingsService.SettingsServiceClient(_channel);
        Players = new PlayerService.PlayerServiceClient(_channel);
        Metrics = new MetricsService.MetricsServiceClient(_channel);
        Mods = new ModService.ModServiceClient(_channel);
        Config = new ConfigService.ConfigServiceClient(_channel);
        Providers = new ProviderService.ProviderServiceClient(_channel);
        Scripts = new ScriptService.ScriptServiceClient(_channel);
        Parsers = new ParserService.ParserServiceClient(_channel);
    }

    public async ValueTask DisposeAsync() => await _channel.ShutdownAsync().ConfigureAwait(false);

    /// <summary>
    /// Connects to the engine's named pipe, returning a <see cref="Stream"/> that
    /// <see cref="SocketsHttpHandler"/> uses as the HTTP/2 transport.
    /// </summary>
    private sealed class NamedPipeConnectionFactory(string pipeName)
    {
        public async ValueTask<Stream> ConnectAsync(
            SocketsHttpConnectionContext context,
            CancellationToken cancellationToken)
        {
            var pipe = new NamedPipeClientStream(
                serverName: ".",
                pipeName: pipeName,
                direction: PipeDirection.InOut,
                options: PipeOptions.WriteThrough | PipeOptions.Asynchronous);

            try
            {
                await pipe.ConnectAsync(cancellationToken).ConfigureAwait(false);
                return pipe;
            }
            catch
            {
                await pipe.DisposeAsync().ConfigureAwait(false);
                throw;
            }
        }
    }
}
