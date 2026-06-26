namespace JustHostMC.Core.Tests;

/// <summary>Shared helpers for locating the engine binary under test.</summary>
internal static class EngineFixture
{
    /// <summary>
    /// Resolves the engine executable copied next to the test assembly. If it is
    /// missing, the error explains how to produce it so the failure is actionable.
    /// </summary>
    public static string EnginePath()
    {
        var path = Path.Combine(AppContext.BaseDirectory, "engine.exe");
        if (!File.Exists(path))
            throw new FileNotFoundException(
                $"engine.exe not found at '{path}'. Build it first: " +
                "from /engine run `go build -o ../build/engine.exe ./cmd/engine`.",
                path);
        return path;
    }

    public static EngineHost NewHost(Action<string>? onDiagnostic = null)
        => new(new EngineHostOptions
        {
            EnginePath = EnginePath(),
            DataDir = Path.Combine(Path.GetTempPath(), "JustHostMC.Tests", Guid.NewGuid().ToString("N")),
            OnDiagnosticLine = onDiagnostic,
        });
}
