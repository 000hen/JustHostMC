namespace JustHostMC.App.Services;

/// <summary>
/// Resolves runtime-only localization keys to user-language strings. Static UI
/// text belongs in XAML via x:Uid; this interface is reserved for backend keys,
/// translator-controlled formats, error mappings, and non-XAML OS surfaces.
/// </summary>
public interface ILocalizer {
    /// <summary>Resolves a dotted key to the current language's string.</summary>
    string Get(string key);

    /// <summary>Resolves a key and substitutes <c>{name}</c>
    /// placeholders.</summary>
    string Get(string key, params(string Name, string Value)[] args);
}
