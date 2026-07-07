using System.Diagnostics;

namespace JustHostMC.Core;

/// <summary>
/// Launches the engine as a child process, waits for its readiness signal, and
/// owns its lifecycle. Closing the engine's stdin (on dispose) signals it to
/// shut down gracefully; a force-kill of the whole tree is the timeout
/// fallback.
/// </summary>
public sealed class EngineHost : IAsyncDisposable {
    private const int MaxStdioHistory  = 5000;
    private const string ReadyLine     = "MCMANAGER_READY";
    private const string PipeEnvVar    = "MCMANAGER_PIPE";
    private const string DataDirEnvVar = "MCMANAGER_DATA_DIR";

    private readonly EngineHostOptions _options;
    private readonly Lock _stdioLock                      = new();
    private readonly List<EngineStdioEntry> _stdioHistory = [];
    private Process? _process;
    private long _stdioSequence;

    public EngineHost(EngineHostOptions options) => _options =
        options ?? throw new ArgumentNullException(nameof(options));

    /// <summary>Set once <see cref="StartAsync"/> has completed the
    /// handshake.</summary>
    public EngineConnection? Connection { get; private set; }

    /// <summary>Process identifier while the engine child process is
    /// available.</summary>
    public int? ProcessId {
        get {
            try {
                return _process?.Id;
            } catch (InvalidOperationException) {
                return null;
            }
        }
    }

    /// <summary>The sequence number of the most recently captured stdio
    /// entry.</summary>
    public long LastStdioSequence => Interlocked.Read(ref _stdioSequence);

    /// <summary>
    /// Raised for every line read from stdout/stderr and for stdin lifecycle
    /// markers. Handlers run on background threads and must marshal UI work
    /// themselves.
    /// </summary>
    public event EventHandler<EngineStdioEntry>? StdioReceived;

    /// <summary>Returns the bounded engine stdio history captured since
    /// launch.</summary>
    public IReadOnlyList<EngineStdioEntry> GetStdioSnapshot() {
        lock (_stdioLock) return [.._stdioHistory];
    }

    /// <summary>Clears captured stdio history without affecting the engine
    /// process.</summary>
    public void ClearStdioHistory() {
        lock (_stdioLock) _stdioHistory.Clear();
    }

    /// <summary>
    /// Starts the engine, passes a generated pipe name via environment
    /// variable, and waits for the engine to signal readiness on stdout.
    /// </summary>
    public async Task<EngineConnection> StartAsync(
        CancellationToken cancellationToken = default) {
        if (_process is not null)
            throw new InvalidOperationException(
                "Engine has already been started.");
        if (!File.Exists(_options.EnginePath))
            throw new FileNotFoundException("Engine executable not found.",
                                            _options.EnginePath);

        var pipeName  = $"JustHostMC-{Guid.NewGuid():N}";
        var startInfo = new ProcessStartInfo {
            FileName = _options.EnginePath, UseShellExecute = false,
            CreateNoWindow = true,          RedirectStandardInput = true,
            RedirectStandardOutput = true,  RedirectStandardError = true,
        };
        startInfo.Environment[PipeEnvVar] = pipeName;
        if (!string.IsNullOrEmpty(_options.DataDir))
            startInfo.Environment[DataDirEnvVar] = _options.DataDir;

        var process =
            new Process { StartInfo = startInfo, EnableRaisingEvents = true };
        if (!process.Start())
            throw new InvalidOperationException(
                "Failed to start the engine process.");
        _process = process;

        RecordStdio(EngineStdioStream.StdIn,
                    "[open] Parent lifecycle watchdog connected.");

        var readySource = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        PumpStandardOutput(process, readySource);
        PumpStandardError(process);

        await WaitForReadyAsync(readySource.Task, cancellationToken)
            .ConfigureAwait(false);
        Connection = new EngineConnection(pipeName);
        return Connection;
    }

    private void PumpStandardOutput(Process process,
                                    TaskCompletionSource readySource) {
        _ = Task.Run(async () => {
            try {
                string? line;
                while ((line = await process.StandardOutput.ReadLineAsync()
                                   .ConfigureAwait(false)) is not null) {
                    RecordStdio(EngineStdioStream.StdOut, line);
                    if (line.Equals(ReadyLine, StringComparison.Ordinal))
                        readySource.TrySetResult();
                }

                readySource.TrySetException(new InvalidOperationException(
                    "Engine exited before signaling readiness."));
            } catch (Exception ex) {
                readySource.TrySetException(ex);
            }
        });
    }

    private void PumpStandardError(Process process) {
        _ = Task.Run(async () => {
            try {
                string? line;
                while ((line = await process.StandardError.ReadLineAsync()
                                   .ConfigureAwait(false)) is not null) {
                    RecordStdio(EngineStdioStream.StdErr, line);
                    try {
                        _options.OnDiagnosticLine?.Invoke(line);
                    } catch { /* diagnostics callbacks are best-effort */
                    }
                }
            } catch {
                // Diagnostics are best-effort; never let them crash the host.
            }
        });
    }

    private async Task WaitForReadyAsync(Task readyTask,
                                         CancellationToken cancellationToken) {
        using var timeoutCts =
            CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        timeoutCts.CancelAfter(_options.StartupTimeout);

        try {
            await readyTask.WaitAsync(timeoutCts.Token).ConfigureAwait(false);
        } catch (OperationCanceledException)
            when (timeoutCts.IsCancellationRequested &&
                  !cancellationToken.IsCancellationRequested) {
            throw new TimeoutException(
                $"Engine did not signal readiness within {_options.StartupTimeout}.");
        }
    }

    private void RecordStdio(EngineStdioStream stream, string message) {
        var entry =
            new EngineStdioEntry(Interlocked.Increment(ref _stdioSequence),
                                 DateTimeOffset.Now, stream, message);

        lock (_stdioLock) {
            _stdioHistory.Add(entry);
            if (_stdioHistory.Count > MaxStdioHistory)
                _stdioHistory.RemoveRange(
                    0, _stdioHistory.Count - MaxStdioHistory);
        }

        try {
            StdioReceived?.Invoke(this, entry);
        } catch { /* observers must never interrupt process IO */
        }
    }

    public async ValueTask DisposeAsync() {
        var process = _process;
        if (process is null)
            return;
        _process = null;

        try {
            if (!process.HasExited) {
                // Closing stdin trips the engine's watchdog -> graceful
                // shutdown.
                try {
                    process.StandardInput.Close();
                    RecordStdio(EngineStdioStream.StdIn,
                                "[EOF] Parent lifecycle watchdog closed.");
                } catch { /* already gone */
                }

                using var cts =
                    new CancellationTokenSource(_options.StopTimeout);
                try {
                    await process.WaitForExitAsync(cts.Token).ConfigureAwait(
                        false);
                } catch (
                    OperationCanceledException) { /* fall through to kill */
                }
            }

            if (!process.HasExited)
                process.Kill(entireProcessTree: true);
        } catch {
            // Best-effort cleanup; nothing actionable if the OS already reaped
            // it.
        } finally {
            process.Dispose();
        }
    }
}
