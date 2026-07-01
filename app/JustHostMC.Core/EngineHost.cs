using System.Diagnostics;

namespace JustHostMC.Core;

/// <summary>
/// Launches the engine as a child process, waits for its readiness signal, and
/// owns its lifecycle. Closing the engine's stdin (on dispose) signals it to
/// shut down gracefully; a force-kill of the whole tree is the timeout fallback.
/// </summary>
public sealed class EngineHost : IAsyncDisposable
{
    private const string ReadyLine = "MCMANAGER_READY";
    private const string PipeEnvVar = "MCMANAGER_PIPE";
    private const string DataDirEnvVar = "MCMANAGER_DATA_DIR";

    private readonly EngineHostOptions _options;
    private Process? _process;

    public EngineHost(EngineHostOptions options)
        => _options = options ?? throw new ArgumentNullException(nameof(options));

    /// <summary>Set once <see cref="StartAsync"/> has completed the handshake.</summary>
    public EngineConnection? Connection { get; private set; }

    /// <summary>
    /// Starts the engine, passes a generated pipe name via environment variable,
    /// and waits for the engine to signal readiness on stdout.
    /// </summary>
    public async Task<EngineConnection> StartAsync(CancellationToken cancellationToken = default)
    {
        if (_process is not null)
            throw new InvalidOperationException("Engine has already been started.");
        if (!File.Exists(_options.EnginePath))
            throw new FileNotFoundException("Engine executable not found.", _options.EnginePath);

        var pipeName = $"JustHostMC-{Guid.NewGuid():N}";
        var startInfo = new ProcessStartInfo
        {
            FileName = _options.EnginePath,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };
        startInfo.Environment[PipeEnvVar] = pipeName;
        if (!string.IsNullOrEmpty(_options.DataDir))
            startInfo.Environment[DataDirEnvVar] = _options.DataDir;

        var process = new Process { StartInfo = startInfo, EnableRaisingEvents = true };
        if (!process.Start())
            throw new InvalidOperationException("Failed to start the engine process.");
        _process = process;

        PumpDiagnostics(process);

        await WaitForReadyAsync(process, cancellationToken).ConfigureAwait(false);
        Connection = new EngineConnection(pipeName);
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

    private async Task WaitForReadyAsync(Process process, CancellationToken cancellationToken)
    {
        using var timeoutCts = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        timeoutCts.CancelAfter(_options.StartupTimeout);

        try
        {
            while (true)
            {
                var line = await process.StandardOutput.ReadLineAsync(timeoutCts.Token).ConfigureAwait(false);
                if (line is null)
                    throw new InvalidOperationException("Engine exited before signaling readiness.");

                if (line.Equals(ReadyLine, StringComparison.Ordinal))
                    return;
            }
        }
        catch (OperationCanceledException) when (timeoutCts.IsCancellationRequested && !cancellationToken.IsCancellationRequested)
        {
            throw new TimeoutException($"Engine did not signal readiness within {_options.StartupTimeout}.");
        }
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
