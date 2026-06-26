using System.Diagnostics;
using System.Security.Cryptography;

namespace JustHostMC.Core;

/// <summary>
/// Launches the engine as a child process, performs the port handshake, and owns
/// its lifecycle. Closing the engine's stdin (on dispose) signals it to shut down
/// gracefully; a force-kill of the whole tree is the timeout fallback.
/// </summary>
public sealed class EngineHost : IAsyncDisposable
{
    private const string PortLinePrefix = "MCMANAGER_PORT=";
    private const string TokenEnvVar = "MCMANAGER_TOKEN";
    private const string DataDirEnvVar = "MCMANAGER_DATA_DIR";

    private readonly EngineHostOptions _options;
    private Process? _process;

    public EngineHost(EngineHostOptions options)
        => _options = options ?? throw new ArgumentNullException(nameof(options));

    /// <summary>Set once <see cref="StartAsync"/> has completed the handshake.</summary>
    public EngineConnection? Connection { get; private set; }

    /// <summary>
    /// Starts the engine, generates a random session token, and waits for the
    /// engine to report its loopback port on stdout.
    /// </summary>
    public async Task<EngineConnection> StartAsync(CancellationToken cancellationToken = default)
    {
        if (_process is not null)
            throw new InvalidOperationException("Engine has already been started.");
        if (!File.Exists(_options.EnginePath))
            throw new FileNotFoundException("Engine executable not found.", _options.EnginePath);

        var token = GenerateToken();
        var startInfo = new ProcessStartInfo
        {
            FileName = _options.EnginePath,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };
        startInfo.Environment[TokenEnvVar] = token;
        if (!string.IsNullOrEmpty(_options.DataDir))
            startInfo.Environment[DataDirEnvVar] = _options.DataDir;

        var process = new Process { StartInfo = startInfo, EnableRaisingEvents = true };
        if (!process.Start())
            throw new InvalidOperationException("Failed to start the engine process.");
        _process = process;

        PumpDiagnostics(process);

        var port = await ReadPortAsync(process, cancellationToken).ConfigureAwait(false);
        Connection = new EngineConnection(port, token);
        return Connection;
    }

    private void PumpDiagnostics(Process process)
    {
        if (_options.OnDiagnosticLine is null)
            return;

        _ = Task.Run(async () =>
        {
            try
            {
                string? line;
                while ((line = await process.StandardError.ReadLineAsync().ConfigureAwait(false)) is not null)
                    _options.OnDiagnosticLine(line);
            }
            catch
            {
                // Diagnostics are best-effort; never let them crash the host.
            }
        });
    }

    private async Task<int> ReadPortAsync(Process process, CancellationToken cancellationToken)
    {
        using var timeoutCts = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        timeoutCts.CancelAfter(_options.StartupTimeout);

        try
        {
            while (true)
            {
                var line = await process.StandardOutput.ReadLineAsync(timeoutCts.Token).ConfigureAwait(false);
                if (line is null)
                    throw new InvalidOperationException("Engine exited before reporting a port.");

                if (line.StartsWith(PortLinePrefix, StringComparison.Ordinal)
                    && int.TryParse(line.AsSpan(PortLinePrefix.Length), out var port))
                    return port;
                // Ignore any unrelated stdout lines before the handshake line.
            }
        }
        catch (OperationCanceledException) when (timeoutCts.IsCancellationRequested && !cancellationToken.IsCancellationRequested)
        {
            throw new TimeoutException($"Engine did not report a port within {_options.StartupTimeout}.");
        }
    }

    private static string GenerateToken()
    {
        Span<byte> bytes = stackalloc byte[32];
        RandomNumberGenerator.Fill(bytes);
        return Convert.ToBase64String(bytes);
    }

    public async ValueTask DisposeAsync()
    {
        var process = _process;
        if (process is null)
            return;
        _process = null;

        try
        {
            if (!process.HasExited)
            {
                // Closing stdin trips the engine's watchdog -> graceful shutdown.
                try { process.StandardInput.Close(); } catch { /* already gone */ }

                using var cts = new CancellationTokenSource(_options.StopTimeout);
                try { await process.WaitForExitAsync(cts.Token).ConfigureAwait(false); }
                catch (OperationCanceledException) { /* fall through to kill */ }
            }

            if (!process.HasExited)
                process.Kill(entireProcessTree: true);
        }
        catch
        {
            // Best-effort cleanup; nothing actionable if the OS already reaped it.
        }
        finally
        {
            process.Dispose();
        }
    }
}
