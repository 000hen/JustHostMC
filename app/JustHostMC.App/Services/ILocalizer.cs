namespace JustHostMC.App.Services;

/// <summary>
/// Resolves dotted semantic localization identifiers (for example,
/// <c>namespace.method.type</c>) to user-language strings. The implementation
/// converts dotted identifiers to MRT paths at the resource-loader boundary,
/// keeping all user-visible text in resource files (PROMPT §14).
/// </summary>
public interface ILocalizer {
    /// <summary>Resolves a dotted semantic identifier to the current
    /// language's string.</summary>
    string Get(string key);

    /// <summary>Resolves a dotted semantic identifier and substitutes
    /// <c>{name}</c> placeholders.</summary>
    string Get(string key, params(string Name, string Value)[] args);
}
