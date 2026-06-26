using Microsoft.Windows.ApplicationModel.Resources;

namespace JustHostMC.App.Services;

/// <summary>
/// <see cref="ILocalizer"/> backed by the Windows App SDK resource system, which
/// resolves strings against the user's language with English as the fallback.
/// </summary>
public sealed class LocalizationService : ILocalizer
{
    private readonly ResourceLoader _loader = new();

    public string Get(string key) => _loader.GetString(NormalizeKey(key));

    public string Get(string key, params (string Name, string Value)[] args)
    {
        var format = _loader.GetString(NormalizeKey(key));
        foreach (var (name, value) in args)
            format = format.Replace("{" + name + "}", value);
        return format;
    }

    // Backend keys use '.' separators ("install.progress.downloading_server").
    // Resource names are flat (underscored) to avoid the resw/PRI treating dots
    // as the x:Uid "Uid.Property" convention, so map '.' -> '_'.
    private static string NormalizeKey(string key) => key.Replace('.', '_');
}
