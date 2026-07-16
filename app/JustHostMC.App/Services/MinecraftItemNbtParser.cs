using System.Globalization;
using System.Text.Encodings.Web;
using System.Text.Json;
using SharpNBT;
using SharpNBT.SNBT;

namespace JustHostMC.App.Services;

public enum NbtDetailKind {
    Default,
    Lore,
    Enchantment,
    Effect,
    Numeric,
    Code,
}

public sealed record NbtDetailEntry(
    string Label, string Value, NbtDetailKind Kind = NbtDetailKind.Default);
public sealed record NbtDetailSection(string Title,
                                        IReadOnlyList<NbtDetailEntry> Entries);

internal sealed record MinecraftItemNbtPresentation(
    string? DisplayName, IReadOnlyList<NbtDetailSection> Sections,
    string FormattedJson, string? ParseError);

/// <summary>Parses item SNBT locally and turns Minecraft components into
/// UI-ready metadata.</summary>
internal static class MinecraftItemNbtParser {
    public static MinecraftItemNbtPresentation Parse(string snbt) {
        if (string.IsNullOrWhiteSpace(snbt))
            return new(null, [], "{}", null);

        try {
            var root = StringNbt.Parse(snbt);
            return Present(root);
        } catch (Exception exception) {
            return new(null, [], snbt, exception.Message);
        }
    }

    public static MinecraftItemNbtPresentation Parse(byte[] binaryNbt,
                                                     string fallbackSnbt) {
        if (binaryNbt.Length == 0)
            return Parse(fallbackSnbt);
        try {
            return Present(ReadBinary(binaryNbt));
        } catch (Exception exception) {
            var fallback = Parse(fallbackSnbt);
            return fallback.ParseError is null
                       ? fallback
                       : fallback with {
                             ParseError =
                                 $"Binary NBT: {exception.Message}; SNBT: {fallback.ParseError}"
                         };
        }
    }

    public static string FormatAsJson(string snbt) {
        try {
            return FormatJson(StringNbt.Parse(snbt), indented: true);
        } catch {
            return snbt;
        }
    }

    public static string FormatAsJson(byte[] binaryNbt, string fallbackSnbt) {
        if (binaryNbt.Length > 0) {
            try {
                return FormatJson(ReadBinary(binaryNbt), indented: true);
            } catch {
                // Fall through to the legacy SNBT payload for compatibility.
            }
        }
        return FormatAsJson(fallbackSnbt);
    }

    private static CompoundTag ReadBinary(byte[] binaryNbt) {
        using var stream = new MemoryStream(binaryNbt, writable: false);
        using var reader =
            new TagReader(stream, FormatOptions.Java, leaveOpen: false);
        return reader.ReadTag(named: true) as CompoundTag ??
               throw new InvalidDataException(
                   "The NBT root is not a compound tag.");
    }

    private static MinecraftItemNbtPresentation Present(CompoundTag root) {
        var sections           = new List<NbtDetailSection>();
        var components         = GetCompound(root, "components");
        var legacy             = GetCompound(root, "tag");
        var consumedComponents = new HashSet<string>(StringComparer.Ordinal);
        var consumedLegacy     = new HashSet<string>(StringComparer.Ordinal);

        var displayName = FirstText(
            Take(components, consumedComponents, "minecraft:custom_name"),
            Take(components, consumedComponents, "minecraft:item_name"),
            GetCompound(legacy, "display") is {} display ? Get(display, "Name")
                                                         : null);

        AddLore(sections, components, legacy, consumedComponents,
                consumedLegacy);
        AddEnchantments(sections, components, legacy, consumedComponents,
                        consumedLegacy);
        AddEffects(sections, components, legacy, consumedComponents,
                   consumedLegacy);
        AddDurability(sections, components, legacy, consumedComponents,
                      consumedLegacy);
        AddAttributes(sections, components, consumedComponents);
        AddKnownComponents(sections, components, consumedComponents);
        AddRemainingComponents(sections, components, consumedComponents,
                               "Components");
        AddRemainingComponents(sections, legacy, consumedLegacy, "Legacy NBT");

        return new(displayName, sections, FormatJson(root, indented: false),
                   null);
    }

    private static void AddLore(List<NbtDetailSection> sections,
                                CompoundTag? components, CompoundTag? legacy,
                                HashSet<string> consumedComponents,
                                HashSet<string> consumedLegacy) {
        var lore =
            Take(components, consumedComponents, "minecraft:lore") ??
            (GetCompound(legacy, "display") is {} display ? Get(display, "Lore")
                                                          : null);
        if (lore is not ListTag list)
            return;
        var lines = list.Select(Text)
                        .Where(value => !string.IsNullOrWhiteSpace(value))
                        .Select(value => new NbtDetailEntry("", value,
                                                            NbtDetailKind.Lore))
                        .ToList();
        AddSection(sections, "Lore", lines);
        consumedLegacy.Add("display");
    }

    private static void AddEnchantments(List<NbtDetailSection> sections,
                                        CompoundTag? components,
                                        CompoundTag? legacy,
                                        HashSet<string> consumedComponents,
                                        HashSet<string> consumedLegacy) {
        var entries = new List<NbtDetailEntry>();
        foreach (var key in new[] { "minecraft:enchantments",
                                    "minecraft:stored_enchantments" }) {
            if (Take(components, consumedComponents, key)
                    is not CompoundTag enchantments)
                continue;
            var levels = GetCompound(enchantments, "levels") ?? enchantments;
            foreach (var tag in levels.Values) {
                if (tag.Name == "show_in_tooltip" ||
                    !TryInteger(tag, out var level))
                    continue;
                entries.Add(new NbtDetailEntry(
                    "", $"{FriendlyId(tag.Name)} {Roman(level)}",
                    NbtDetailKind.Enchantment));
            }
        }
        foreach (var key in new[] { "Enchantments", "StoredEnchantments" }) {
            consumedLegacy.Add(key);
            if (Get(legacy, key) is not ListTag list)
                continue;
            foreach (var tag in list.OfType<CompoundTag>()) {
                var id = StringValue(Get(tag, "id"));
                if (!string.IsNullOrWhiteSpace(id) &&
                    TryInteger(Get(tag, "lvl"), out var level))
                    entries.Add(new NbtDetailEntry(
                        "", $"{FriendlyId(id)} {Roman(level)}",
                        NbtDetailKind.Enchantment));
            }
        }
        AddSection(sections, "Enchantments", entries);
    }

    private static void AddEffects(List<NbtDetailSection> sections,
                                   CompoundTag? components, CompoundTag? legacy,
                                   HashSet<string> consumedComponents,
                                   HashSet<string> consumedLegacy) {
        var entries = new List<NbtDetailEntry>();
        if (Take(components, consumedComponents, "minecraft:potion_contents")
                is CompoundTag potion) {
            var basePotion = StringValue(Get(potion, "potion"));
            if (!string.IsNullOrWhiteSpace(basePotion))
                entries.Add(new NbtDetailEntry("Potion", FriendlyId(basePotion),
                                               NbtDetailKind.Effect));
            AddEffectList(entries, Get(potion, "custom_effects"));
        }
        AddEffectList(entries, Take(components, consumedComponents,
                                    "minecraft:suspicious_stew_effects"));
        consumedLegacy.Add("Potion");
        var legacyPotion = StringValue(Get(legacy, "Potion"));
        if (!string.IsNullOrWhiteSpace(legacyPotion))
            entries.Add(new NbtDetailEntry("Potion", FriendlyId(legacyPotion),
                                           NbtDetailKind.Effect));
        consumedLegacy.Add("CustomPotionEffects");
        AddEffectList(entries, Get(legacy, "CustomPotionEffects"));
        AddSection(sections, "Effects", entries);
    }

    private static void AddEffectList(List<NbtDetailEntry> entries,
                                      Tag? value) {
        if (value is not ListTag list)
            return;
        foreach (var effect in list.OfType<CompoundTag>()) {
            var id = StringValue(Get(effect, "id")) ??
                     StringValue(Get(effect, "effect"));
            if (string.IsNullOrWhiteSpace(id) &&
                TryInteger(Get(effect, "Id"), out var numericId))
                id = $"Effect {numericId}";
            if (string.IsNullOrWhiteSpace(id))
                continue;
            var text = FriendlyId(id);
            if ((TryInteger(Get(effect, "amplifier"), out var amplifier) ||
                 TryInteger(Get(effect, "Amplifier"), out amplifier)) &&
                amplifier > 0)
                text += $" {Roman(amplifier + 1)}";
            if (TryInteger(Get(effect, "duration"), out var duration) ||
                TryInteger(Get(effect, "Duration"), out duration))
                text += $" ({duration / 1200}:{duration / 20 % 60:00})";
            entries.Add(new NbtDetailEntry("", text, NbtDetailKind.Effect));
        }
    }

    private static void AddDurability(List<NbtDetailSection> sections,
                                      CompoundTag? components,
                                      CompoundTag? legacy,
                                      HashSet<string> consumedComponents,
                                      HashSet<string> consumedLegacy) {
        var entries = new List<NbtDetailEntry>();
        AddNumber(entries, "Damage",
                  Take(components, consumedComponents, "minecraft:damage") ??
                      Get(legacy, "Damage"));
        consumedLegacy.Add("Damage");
        AddNumber(entries, "Maximum damage",
                  Take(components, consumedComponents, "minecraft:max_damage"));
        AddNumber(
            entries, "Repair cost",
            Take(components, consumedComponents, "minecraft:repair_cost") ??
                Get(legacy, "RepairCost"));
        consumedLegacy.Add("RepairCost");
        if (Take(components, consumedComponents, "minecraft:unbreakable")
                is not null ||
            IsTrue(Get(legacy, "Unbreakable")))
            entries.Add(new NbtDetailEntry("Unbreakable", "Yes"));
        consumedLegacy.Add("Unbreakable");
        AddSection(sections, "Durability", entries);
    }

    private static void AddAttributes(List<NbtDetailSection> sections,
                                      CompoundTag? components,
                                      HashSet<string> consumed) {
        var component =
            Take(components, consumed, "minecraft:attribute_modifiers");
        var list = component switch {
            ListTag direct       => direct,
            CompoundTag compound => Get(compound, "modifiers") as ListTag,
            _                    => null,
        };
        if (list is null)
            return;
        var entries = new List<NbtDetailEntry>();
        foreach (var modifier in list.OfType<CompoundTag>()) {
            var type   = StringValue(Get(modifier, "type")) ??
                         StringValue(Get(modifier, "attribute")) ?? "Attribute";
            var amount = Scalar(Get(modifier, "amount"));
            var operation = StringValue(Get(modifier, "operation"));
            entries.Add(new NbtDetailEntry(
                FriendlyId(type),
                string.Join(' ',
                            new[] { amount, operation }.Where(
                                value => !string.IsNullOrWhiteSpace(value)))));
        }
        AddSection(sections, "Attributes", entries);
    }

    private static void AddKnownComponents(List<NbtDetailSection> sections,
                                           CompoundTag? components,
                                           HashSet<string> consumed) {
        var entries = new List<NbtDetailEntry>();
        AddComponent(entries, components, consumed,
                     "minecraft:custom_model_data", "Custom model data");
        AddComponent(entries, components, consumed, "minecraft:rarity",
                     "Rarity");
        AddComponent(entries, components, consumed, "minecraft:trim",
                     "Armor trim");
        AddComponent(entries, components, consumed, "minecraft:profile",
                     "Profile");
        AddComponent(entries, components, consumed, "minecraft:container",
                     "Container");
        AddComponent(entries, components, consumed, "minecraft:container_loot",
                     "Container loot");
        AddComponent(entries, components, consumed, "minecraft:instrument",
                     "Instrument");
        AddComponent(entries, components, consumed,
                     "minecraft:written_book_content", "Book");
        AddComponent(entries, components, consumed,
                     "minecraft:writable_book_content", "Book");
        AddSection(sections, "Item components", entries);

        if (Take(components, consumed, "minecraft:custom_data")
                is {} customData)
            AddSection(sections, "Custom data", Flatten(customData, ""));
    }

    private static void AddRemainingComponents(List<NbtDetailSection> sections,
                                               CompoundTag? source,
                                               HashSet<string> consumed,
                                               string title) {
        if (source is null)
            return;
        var entries =
            source.Values
                .Where(tag => tag.Name is null || !consumed.Contains(tag.Name))
                .Select(tag => new NbtDetailEntry(FriendlyId(tag.Name),
                                                  Compact(tag),
                                                  NbtDetailKind.Code))
                .ToList();
        AddSection(sections, title, entries);
    }

    private static List<NbtDetailEntry> Flatten(Tag tag, string path) {
        var entries = new List<NbtDetailEntry>();
        if (tag is CompoundTag compound) {
            foreach (var child in compound.Values) {
                var childPath = string.IsNullOrEmpty(path)
                                    ? FriendlyId(child.Name)
                                    : $"{path} › {FriendlyId(child.Name)}";
                entries.AddRange(Flatten(child, childPath));
            }
        } else if (tag is ListTag list) {
            for (var index = 0; index < list.Count; index++)
                entries.AddRange(Flatten(list[index], $"{path} [{index}]"));
        } else {
            entries.Add(
                new NbtDetailEntry(path, Scalar(tag), NbtDetailKind.Code));
        }
        return entries;
    }

    private static void AddComponent(List<NbtDetailEntry> entries,
                                     CompoundTag? source,
                                     HashSet<string> consumed, string key,
                                     string label) {
        if (Take(source, consumed, key) is {} value)
            entries.Add(
                new NbtDetailEntry(label, Compact(value), NbtDetailKind.Code));
    }

    private static void AddNumber(List<NbtDetailEntry> entries, string label,
                                  Tag? tag) {
        if (tag is not null)
            entries.Add(
                new NbtDetailEntry(label, Scalar(tag), NbtDetailKind.Numeric));
    }

    private static void AddSection(List<NbtDetailSection> sections,
                                   string title,
                                   IReadOnlyList<NbtDetailEntry> entries) {
        if (entries.Count > 0)
            sections.Add(new NbtDetailSection(title, entries));
    }

    private static Tag? Take(CompoundTag? source, HashSet<string> consumed,
                             string key) {
        consumed.Add(key);
        return Get(source, key);
    }

    private static Tag? Get(CompoundTag? source, string key) =>
        source is not null && source.TryGetValue(key, out var value) ? value
                                                                     : null;

    private static CompoundTag? GetCompound(CompoundTag? source, string key) =>
        Get(source, key) as CompoundTag;

    private static string? FirstText(params Tag?[] values) =>
        values.Select(Text).FirstOrDefault(
            value => !string.IsNullOrWhiteSpace(value));

    private static string Text(Tag? tag) {
        if (tag is null)
            return "";
        if (tag is StringTag text)
            return JsonText(text.Value);
        if (tag is ListTag list)
            return string.Concat(list.Select(Text));
        if (tag is CompoundTag compound) {
            var value = StringValue(Get(compound, "text")) ??
                        StringValue(Get(compound, "translate")) ?? "";
            if (Get(compound, "extra") is ListTag extra)
                value += string.Concat(extra.Select(Text));
            return value;
        }
        return Scalar(tag);
    }

    private static string JsonText(string value) {
        try {
            using var document = JsonDocument.Parse(value);
            return JsonText(document.RootElement);
        } catch (JsonException) {
            return value;
        }
    }

    private static string JsonText(
        JsonElement element) => element.ValueKind switch {
        JsonValueKind.String => element.GetString() ?? "",
        JsonValueKind.Array =>
            string.Concat(element.EnumerateArray().Select(JsonText)),
        JsonValueKind.Object =>
            (element.TryGetProperty("text", out var text) ? JsonText(text)
             : element.TryGetProperty("translate", out var translate)
                 ? JsonText(translate)
                 : "") +
            (element.TryGetProperty("extra", out var extra) ? JsonText(extra)
                                                            : ""),
        _ => element.ToString(),
    };

    private static string? StringValue(Tag? tag) => tag is StringTag value
                                                        ? value.Value
                                                        : null;

    private static bool TryInteger(Tag? tag, out long value) {
        switch (tag) {
            case ByteTag number:
                value = number.SignedValue;
                return true;
            case ShortTag number:
                value = number.Value;
                return true;
            case IntTag number:
                value = number.Value;
                return true;
            case LongTag number:
                value = number.Value;
                return true;
            default:
                value = 0;
                return false;
        }
    }

    private static bool IsTrue(Tag? tag) => tag is CompoundTag ||
                                            (TryInteger(tag, out var value) &&
                                             value != 0);

    private static string Scalar(Tag? tag) => tag switch {
        null            => "",
        StringTag value => value.Value,
        ByteTag value =>
            value.SignedValue.ToString(CultureInfo.InvariantCulture),
        ShortTag value  => value.Value.ToString(CultureInfo.InvariantCulture),
        IntTag value    => value.Value.ToString(CultureInfo.InvariantCulture),
        LongTag value   => value.Value.ToString(CultureInfo.InvariantCulture),
        FloatTag value  => value.Value.ToString(CultureInfo.InvariantCulture),
        DoubleTag value => value.Value.ToString(CultureInfo.InvariantCulture),
        _               => Compact(tag),
    };

    private static string Compact(Tag tag) {
        if (tag is StringTag or ByteTag or ShortTag or IntTag or LongTag or
                FloatTag or DoubleTag)
            return Scalar(tag);
        try {
            return JsonSerializer.Serialize(ToJsonValue(tag),
                                            JsonOptions(false));
        } catch {
            return tag.ToString() ?? "";
        }
    }

    private static string FormatJson(CompoundTag root, bool indented) {
        return JsonSerializer.Serialize(ToJsonValue(root),
                                        JsonOptions(indented));
    }

    private static JsonSerializerOptions JsonOptions(bool indented) => new() {
        WriteIndented = indented,
        Encoder       = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };

    private static object? ToJsonValue(Tag tag) {
        switch (tag) {
            case CompoundTag compound:
                return compound.Values.ToDictionary(child => child.Name ?? "",
                                                    ToJsonValue);
            case ListTag list:
                return list.Select(ToJsonValue).ToList();
            case StringTag value:
                return value.Value;
            case ByteTag value:
                return value.SignedValue;
            case ShortTag value:
                return value.Value;
            case IntTag value:
                return value.Value;
            case LongTag value:
                return value.Value;
            case FloatTag value:
                return value.Value;
            case DoubleTag value:
                return value.Value;
            default:
                using (var document = JsonDocument.Parse(tag.ToJson())) {
                    var element = document.RootElement;
                    if (element.ValueKind == JsonValueKind.Array &&
                        element.GetArrayLength() == 1)
                        element = element[0];
                    return element.Clone();
                }
        }
    }

    private static string FriendlyId(string? value) {
        if (string.IsNullOrWhiteSpace(value))
            return "Value";
        var separator = value.IndexOf(':');
        if (separator >= 0)
            value = value[(separator + 1)..];
        return string.Join(
            ' ',
            value.Split('_', StringSplitOptions.RemoveEmptyEntries)
                .Select(word => char.ToUpperInvariant(word[0]) + word[1..]));
    }

    private static string Roman(long value) => value switch {
        1  => "I",
        2  => "II",
        3  => "III",
        4  => "IV",
        5  => "V",
        6  => "VI",
        7  => "VII",
        8  => "VIII",
        9  => "IX",
        10 => "X",
        _  => value.ToString(CultureInfo.InvariantCulture),
    };
}
