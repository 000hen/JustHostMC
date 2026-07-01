namespace JustHostMC.Core;

/// <summary>
/// Connection details for a running engine process: the named pipe it is
/// listening on.
/// </summary>
public sealed record EngineConnection(string PipeName);
