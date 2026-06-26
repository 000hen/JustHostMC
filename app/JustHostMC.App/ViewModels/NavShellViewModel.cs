using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>Backs the navigation shell. Owns the shared <see cref="MainViewModel"/>
/// (the single source of the live server list) and raises navigation requests that
/// the shell window fulfills by changing the NavigationView selection.</summary>
public sealed class NavShellViewModel : ObservableObject
{
    private readonly Dictionary<string, ServerViewModelCache> _serverVmCache = new();

    public NavShellViewModel(MainViewModel main) => Main = main;

    public MainViewModel Main { get; }

    /// <summary>Raised when a page (e.g. a Home card) asks to open a server's page.</summary>
    public event Action<ServerItem>? OpenServerRequested;

    /// <summary>Raised when a page asks to return to Home (e.g. after deleting a server).</summary>
    public event Action? HomeRequested;

    public void RequestOpenServer(ServerItem server) => OpenServerRequested?.Invoke(server);

    public void RequestHome() => HomeRequested?.Invoke();

    /// <summary>Returns a cached set of VMs for the given server, creating them on first access.
    /// Cached VMs keep their gRPC streams alive across page navigations.</summary>
    public ServerViewModelCache GetOrCreateServerCache(
        string serverId, string serverName,
        DispatcherQueue dispatcher, ILocalizer localizer)
    {
        if (!_serverVmCache.TryGetValue(serverId, out var cache))
        {
            cache = new ServerViewModelCache(serverId, serverName, dispatcher, localizer);
            _serverVmCache[serverId] = cache;

            var tracker = Main.ProgressService.GetOrCreateTracker(serverId, serverName);
            foreach (var line in tracker.InstallLog)
                cache.Console.AppendExternalLine(line);
            tracker.LogAppended += line => cache.Console.AppendExternalLine(line);
        }
        else
        {
            cache.Console.ServerName = serverName;
        }
        return cache;
    }

    /// <summary>Evicts and disposes the cached VMs for a removed server.</summary>
    public async Task EvictServerCacheAsync(string serverId)
    {
        if (_serverVmCache.Remove(serverId, out var cache))
            await cache.DisposeAsync();
    }
}
