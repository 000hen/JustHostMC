using JustHostMC.App.ViewModels;

namespace JustHostMC.App.Models;

/// <summary>Navigation parameter for the server page: which server to show plus the
/// shell (for shared commands and navigating back after delete).</summary>
public sealed record ServerPageArgs(ServerItem Server, NavShellViewModel Shell);
