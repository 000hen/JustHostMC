namespace JustHostMC.Core;

/// <summary>
/// Connection details for a running engine process: the loopback port it is
/// listening on and the per-launch session token required on every gRPC call.
/// </summary>
public sealed record EngineConnection(int Port, string Token);
