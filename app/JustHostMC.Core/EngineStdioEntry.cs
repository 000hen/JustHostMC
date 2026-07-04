namespace JustHostMC.Core;

/// <summary>Identifies one of the redirected standard streams on the engine process.</summary>
public enum EngineStdioStream
{
    StdOut,
    StdErr,
    StdIn,
}

/// <summary>A timestamped line or lifecycle marker observed on an engine standard stream.</summary>
public sealed record EngineStdioEntry(
    long Sequence,
    DateTimeOffset Timestamp,
    EngineStdioStream Stream,
    string Message);
