namespace JustHostMC.Core;

/// <summary>Identifies one of the redirected standard streams on the engine process.</summary>
public enum EngineStdioStream
{
    StdOut,
    StdErr,
    StdIn,
}

/// <summary>Severity assigned by the Go engine's diagnostic log prefix.</summary>
public enum EngineDiagnosticLevel
{
    Debug,
    Information,
    Warning,
    Error,
}

/// <summary>A timestamped line or lifecycle marker observed on an engine standard stream.</summary>
public sealed record EngineStdioEntry(
    long Sequence,
    DateTimeOffset Timestamp,
    EngineStdioStream Stream,
    string Message)
{
    private static readonly string[] SeverityMarkers =
        ["[DEBUG]", "[INFO]", "[WARN]", "[ERROR]", "[FATAL]"];

    /// <summary>
    /// Parses the engine's explicit severity marker. Untagged stderr is treated
    /// as information because Go deliberately writes all process logs there.
    /// </summary>
    public EngineDiagnosticLevel Level
    {
        get
        {
            if (Message.Contains("[FATAL]", StringComparison.OrdinalIgnoreCase)
                || Message.Contains("[ERROR]", StringComparison.OrdinalIgnoreCase))
                return EngineDiagnosticLevel.Error;
            if (Message.Contains("[WARN]", StringComparison.OrdinalIgnoreCase))
                return EngineDiagnosticLevel.Warning;
            if (Message.Contains("[DEBUG]", StringComparison.OrdinalIgnoreCase)
                || Stream == EngineStdioStream.StdIn)
                return EngineDiagnosticLevel.Debug;
            return EngineDiagnosticLevel.Information;
        }
    }

    /// <summary>Message without the duplicate Go logger timestamp/severity prefix.</summary>
    public string DisplayMessage
    {
        get
        {
            foreach (var marker in SeverityMarkers)
            {
                var markerIndex = Message.IndexOf(marker, StringComparison.OrdinalIgnoreCase);
                if (markerIndex is >= 0 and <= 32)
                    return Message[(markerIndex + marker.Length)..].TrimStart();
            }

            return Message;
        }
    }
}
