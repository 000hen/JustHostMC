namespace JustHostMC.App.Services;

/// <summary>
/// Resolves localization keys to user-language strings. Backend keys
/// ("namespace.method.type") and resource keys both resolve here, keeping all
/// user-visible text in resource files (PROMPT §14).
/// </summary>
public interface ILocalizer
{
    /// <summary>Resolves a key to the current language's string.</summary>
    string Get(string key);

    /// <summary>Resolves a key and substitutes <c>{name}</c> placeholders.</summary>
    string Get(string key, params (string Name, string Value)[] args);
}
