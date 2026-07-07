namespace JustHostMC.Core;

/// <summary>Configuration for launching and supervising the engine child
/// process.</summary>
public sealed class EngineHostOptions {
    /// <summary>Absolute path to the bundled <c>engine.exe</c>.</summary>
    public required string EnginePath { get; init; }

    /// <summary>
    /// Optional override for the engine's data directory (servers, JRE cache,
    /// logs, backups, registry). When the app is packaged this is set to the
    /// package's local store so a clean uninstall removes all data (PROMPT §8).
    /// When null, the engine falls back to %LOCALAPPDATA%\JustHostMC.
    /// </summary>
    public string? DataDir { get; init; }

    /// <summary>How long to wait for the engine to report its port before
    /// failing.</summary>
    public TimeSpan StartupTimeout { get; init; } = TimeSpan.FromSeconds(15);

    /// <summary>How long to wait for graceful shutdown before force-killing the
    /// tree.</summary>
    public TimeSpan StopTimeout { get; init; } = TimeSpan.FromSeconds(10);

    /// <summary>Optional sink for the engine's stderr diagnostic
    /// lines.</summary>
    public Action<string>? OnDiagnosticLine { get; init; }
}
