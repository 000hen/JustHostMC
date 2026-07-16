using System.Collections.ObjectModel;
using System.Text;
using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Editable server.properties or gamerule row with type
/// metadata.</summary>
public sealed partial class ConfigEntryItem : ObservableObject {
    private readonly string _originalValue;

    public ConfigEntryItem(ConfigEntry entry, ILocalizer localizer) {
        Key            = entry.Key;
        DisplayName    = ResolveDisplayName(localizer, entry.Key);
        _originalValue = entry.Value;
        Value          = entry.Value;
        Type           = entry.Type;
        Supported      = entry.Supported;
        SinceVersion   = entry.SinceVersion;
        Description    = entry.Description;
        foreach (var choice in entry.Choices) Choices.Add(choice);
    }

    public string Key { get; }
    public string DisplayName { get; }
    public ConfigValueType Type { get; }
    public bool IsBoolean => Type == ConfigValueType.ConfigBoolean;
    public bool IsInteger => Type == ConfigValueType.ConfigInteger;
    public bool IsChoice => Type == ConfigValueType.ConfigEnum;
    public bool IsText => !IsBoolean && !IsInteger && !IsChoice;
    public bool Supported { get; }
    public string SinceVersion { get; }
    public string Description { get; }
    public ObservableCollection<string> Choices { get; } = new();
    public bool HasDescription => !string.IsNullOrWhiteSpace(Description);

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(IsModified))]
    public partial string Value { get; set; }

    public bool IsModified =>
        !string.Equals(Value, _originalValue, System.StringComparison.Ordinal);

    public void DiscardChanges() => Value = _originalValue;

    public bool HasChoices => Choices.Count > 0;

    public ConfigUpdate ToUpdate() => new() {
        Key   = Key,
        Value = Value ?? "",
    };

    private static string ResolveDisplayName(ILocalizer localizer, string key) {
        var localized = SafeGet(localizer, "ConfigName_" + ResourceKey(key));
        return string.IsNullOrWhiteSpace(localized) ? HumanizeKey(key)
                                                    : localized;
    }

    private static string SafeGet(ILocalizer localizer, string key) {
        try {
            return localizer.Get(key);
        } catch {
            return "";
        }
    }

    private static string ResourceKey(string key) {
        var b = new StringBuilder(key.Length);
        foreach (var ch in key) b.Append(char.IsLetterOrDigit(ch) ? ch : '_');
        return b.ToString();
    }

    private static string HumanizeKey(string key) {
        key   = key.Replace('.', ' ').Replace('-', ' ').Replace('_', ' ');
        var b = new StringBuilder(key.Length + 8);
        for (var i = 0; i < key.Length; i++) {
            var ch = key[i];
            if (i > 0 && char.IsUpper(ch) && char.IsLower(key[i - 1]))
                b.Append(' ');
            b.Append(ch);
        }
        var text = b.ToString().Trim();
        return text.Length == 0 ? key
                                : char.ToUpperInvariant(text[0]) + text[1..];
    }
}
