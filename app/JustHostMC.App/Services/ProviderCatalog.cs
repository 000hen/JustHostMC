using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using McManager.Grpc;

namespace JustHostMC.App.Services;

/// <summary>
/// Caches the installed provider list (built-in + user-imported) and exposes
/// synchronous lookups by provider id, so UI can resolve friendly names and
/// capabilities without re-fetching. Populated once from the engine; raises
/// <see cref="Loaded"/> when the cache first becomes available so bound items
/// (e.g. <c>ServerItem.TypeText</c>) can refresh.
/// </summary>
public sealed class ProviderCatalog
{
    private readonly Func<Task<IReadOnlyList<ProviderInfo>>> _fetch;
    private readonly object _gate = new();
    private Dictionary<string, ProviderInfo> _byId = new();
    private Task<IReadOnlyList<ProviderInfo>>? _load;

    public ProviderCatalog(Func<Task<IReadOnlyList<ProviderInfo>>> fetch) => _fetch = fetch;

    /// <summary>Raised on the loading thread once the catalog is first populated.</summary>
    public event Action? Loaded;

    /// <summary>True once the catalog has been populated at least once.</summary>
    public bool IsLoaded { get; private set; }

    /// <summary>Loads (once) and returns the provider list; subsequent calls reuse the cache.</summary>
    public Task<IReadOnlyList<ProviderInfo>> GetAllAsync()
    {
        lock (_gate)
        {
            _load ??= LoadAsync();
            return _load;
        }
    }

    private async Task<IReadOnlyList<ProviderInfo>> LoadAsync()
    {
        IReadOnlyList<ProviderInfo> providers;
        try
        {
            providers = await _fetch();
        }
        catch
        {
            // Let the cache stay empty and retry on the next call.
            lock (_gate) _load = null;
            throw;
        }

        lock (_gate)
        {
            _byId = providers
                .Where(p => !string.IsNullOrEmpty(p.Id))
                .GroupBy(p => p.Id)
                .ToDictionary(g => g.Key, g => g.First());
            IsLoaded = true;
        }

        Loaded?.Invoke();
        return providers;
    }

    /// <summary>Returns the cached provider for an id, or null if not yet loaded / unknown.</summary>
    public ProviderInfo? Find(string id)
    {
        if (string.IsNullOrEmpty(id))
            return null;
        lock (_gate)
            return _byId.TryGetValue(id, out var info) ? info : null;
    }

    /// <summary>The provider's friendly name, or null when unresolved.</summary>
    public string? NameFor(string id)
    {
        var info = Find(id);
        return string.IsNullOrEmpty(info?.Name) ? null : info!.Name;
    }

    /// <summary>
    /// The provider's mod layout ("plugins" | "mods" | "none"), or null when the
    /// catalog has not yet loaded the given id (UI should assume supported until known).
    /// </summary>
    public string? ModLayoutFor(string id) => Find(id)?.Capabilities?.ModLayout;
}
