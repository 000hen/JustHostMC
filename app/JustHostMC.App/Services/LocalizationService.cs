using System.Diagnostics;
using Microsoft.Windows.ApplicationModel.Resources;

namespace JustHostMC.App.Services;

/// <summary>
/// <see cref="ILocalizer"/> backed by the Windows App SDK resource system,
/// which resolves strings against the user's language with English as the
/// fallback.
/// </summary>
public sealed class LocalizationService : ILocalizer {
    private readonly ResourceLoader _loader = new();

    public string Get(string key) {
        try {
            var value = _loader.GetString(NormalizeKey(key));
            if (!string.IsNullOrEmpty(value))
                return value;
            Debug.WriteLine($"Missing localization resource: {key}");
            return key;
        } catch (Exception ex) {
            Debug.WriteLine($"Failed to load localization resource '{key}': {ex}");
            return key;
        }
    }

    public string Get(string key, params(string Name, string Value)[] args) {
        string format;
        try {
            format = _loader.GetString(NormalizeKey(key));
        } catch (Exception ex) {
            Debug.WriteLine($"Failed to load localization resource '{key}': {ex}");
            format = key;
        }
        foreach (var (name, value) in args)
            format = format.Replace("{" + name + "}", value);
        return format;
    }

    // MRT Core exposes segmented resource identifiers as slash-separated paths.
    // x:Uid property identifiers are resolved by XAML and never pass through
    // this programmatic lookup path.
    internal static string NormalizeKey(string key) => key.Replace('.', '/');
}
