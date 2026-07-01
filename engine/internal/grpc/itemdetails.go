package grpcsvc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/nbt/dynbt"
)

func extractItemDetails(raw nbt.RawMessage) []*mcmanagerv1.PlayerItemDetail {
	var root dynbt.Value
	if err := raw.Unmarshal(&root); err != nil {
		return nil
	}
	details := make([]*mcmanagerv1.PlayerItemDetail, 0, 12)
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			details = append(details, &mcmanagerv1.PlayerItemDetail{Label: label, Value: value})
		}
	}

	components := root.Get("components")
	legacyTag := root.Get("tag")
	add("Custom name", firstText(
		getString(components, "minecraft:custom_name"),
		getString(components, "minecraft:item_name"),
		getString(legacyTag, "display", "Name"),
	))
	if lore := firstValue(getValue(components, "minecraft:lore"), getValue(legacyTag, "display", "Lore")); lore != nil {
		lines := make([]string, 0, len(lore.List()))
		for _, line := range lore.List() {
			if text := plainMinecraftText(line.String()); text != "" {
				lines = append(lines, text)
			}
		}
		add("Lore", strings.Join(lines, "\n"))
	}

	if enchantments := enchantmentDetails(components, legacyTag); len(enchantments) > 0 {
		add("Enchantments", strings.Join(enchantments, "\n"))
	}
	if effects := effectDetails(components, legacyTag); len(effects) > 0 {
		add("Effects", strings.Join(effects, "\n"))
	}
	addNumber := func(label string, values ...*dynbt.Value) {
		for _, value := range values {
			if number, ok := nbtNumber(value); ok {
				add(label, strconv.FormatInt(number, 10))
				return
			}
		}
	}
	addNumber("Damage", getValue(components, "minecraft:damage"), getValue(legacyTag, "Damage"))
	addNumber("Max damage", getValue(components, "minecraft:max_damage"))
	addNumber("Repair cost", getValue(components, "minecraft:repair_cost"), getValue(legacyTag, "RepairCost"))
	addNumber("Custom model data", getValue(components, "minecraft:custom_model_data"), getValue(legacyTag, "CustomModelData"))

	if value := firstValue(getValue(components, "minecraft:unbreakable"), getValue(legacyTag, "Unbreakable")); value != nil {
		if enabled := nbtBoolean(value); enabled {
			add("Unbreakable", "Yes")
		}
	}
	add("Potion", firstString(
		getString(components, "minecraft:potion_contents", "potion"),
		getString(legacyTag, "Potion"),
	))
	add("Trim", compactValue(getValue(components, "minecraft:trim")))
	add("Profile", compactValue(getValue(components, "minecraft:profile")))
	add("Container", listSummary(getValue(components, "minecraft:container")))
	if components != nil && components.TagType() == nbt.TagCompound {
		add("Components", fmt.Sprintf("%d component(s)", components.Compound().Len()))
	}
	for _, detail := range rawComponentDetails(raw) {
		add(detail.Label, detail.Value)
	}
	return details
}

func rawComponentDetails(raw nbt.RawMessage) []*mcmanagerv1.PlayerItemDetail {
	var item struct {
		Components map[string]nbt.RawMessage `nbt:"components"`
		LegacyTag  map[string]nbt.RawMessage `nbt:"tag"`
	}
	if err := raw.Unmarshal(&item); err != nil {
		return nil
	}
	details := make([]*mcmanagerv1.PlayerItemDetail, 0, len(item.Components)+len(item.LegacyTag))
	appendValues := func(prefix string, values map[string]nbt.RawMessage) {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			label := strings.TrimPrefix(key, "minecraft:")
			label = friendlyMinecraftID(label)
			details = append(details, &mcmanagerv1.PlayerItemDetail{
				Label: prefix + label,
				Value: values[key].String(),
			})
		}
	}
	appendValues("Component · ", item.Components)
	appendValues("NBT · ", item.LegacyTag)
	return details
}

func effectDetails(components, legacyTag *dynbt.Value) []string {
	var result []string
	for _, value := range []*dynbt.Value{
		getValue(components, "minecraft:potion_contents", "custom_effects"),
		getValue(components, "minecraft:suspicious_stew_effects"),
		getValue(legacyTag, "CustomPotionEffects"),
	} {
		if value == nil || value.TagType() != nbt.TagList {
			continue
		}
		for _, entry := range value.List() {
			result = appendEffectDetail(result, entry)
		}
	}

	// Food effects wrap the actual effect compound in an `effect` field.
	foodEffects := getValue(components, "minecraft:food", "effects")
	if foodEffects != nil && foodEffects.TagType() == nbt.TagList {
		for _, entry := range foodEffects.List() {
			if nested := getValue(entry, "effect"); nested != nil {
				result = appendEffectDetail(result, nested)
			}
		}
	}
	return uniqueStrings(result)
}

func appendEffectDetail(result []string, entry *dynbt.Value) []string {
	if entry == nil || entry.TagType() != nbt.TagCompound {
		return result
	}
	id := firstString(getString(entry, "id"), getString(entry, "effect"))
	if id == "" {
		if numericID, ok := nbtNumber(getValue(entry, "Id")); ok {
			id = fmt.Sprintf("Effect %d", numericID)
		}
	}
	if id == "" {
		return result
	}

	name := friendlyMinecraftID(id)
	amplifier, hasAmplifier := nbtNumber(firstValue(getValue(entry, "amplifier"), getValue(entry, "Amplifier")))
	if hasAmplifier && amplifier > 0 {
		name += " " + romanNumeral(amplifier+1)
	}
	duration, hasDuration := nbtNumber(firstValue(getValue(entry, "duration"), getValue(entry, "Duration")))
	if hasDuration && duration >= 0 {
		seconds := duration / 20
		name += fmt.Sprintf(" (%d:%02d)", seconds/60, seconds%60)
	}
	return append(result, name)
}

func friendlyMinecraftID(value string) string {
	if _, path, found := strings.Cut(value, ":"); found {
		value = path
	}
	words := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for index := range words {
		words[index] = strings.ToUpper(words[index][:1]) + words[index][1:]
	}
	return strings.Join(words, " ")
}

func romanNumeral(value int64) string {
	if value < 1 || value > 10 {
		return strconv.FormatInt(value, 10)
	}
	numerals := []string{"", "I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X"}
	return numerals[value]
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := values[:0]
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func enchantmentDetails(components, legacyTag *dynbt.Value) []string {
	levels := make(map[string]int32)
	for _, value := range []*dynbt.Value{
		getValue(components, "minecraft:enchantments", "levels"),
		getValue(components, "minecraft:stored_enchantments", "levels"),
	} {
		decodeDynamic(value, &levels)
	}
	result := make([]string, 0, len(levels))
	for id, level := range levels {
		result = append(result, fmt.Sprintf("%s %d", id, level))
	}
	for _, key := range []string{"Enchantments", "StoredEnchantments"} {
		value := getValue(legacyTag, key)
		if value == nil || value.TagType() != nbt.TagList {
			continue
		}
		for _, entry := range value.List() {
			id := getString(entry, "id")
			level, _ := nbtNumber(getValue(entry, "lvl"))
			if id != "" {
				result = append(result, fmt.Sprintf("%s %d", id, level))
			}
		}
	}
	sort.Strings(result)
	return result
}

func decodeDynamic(value *dynbt.Value, target any) bool {
	if value == nil {
		return false
	}
	var buffer bytes.Buffer
	if err := nbt.NewEncoder(&buffer).Encode(value, ""); err != nil {
		return false
	}
	return nbt.Unmarshal(buffer.Bytes(), target) == nil
}

func getValue(root *dynbt.Value, keys ...string) *dynbt.Value {
	if root == nil {
		return nil
	}
	return root.Get(keys...)
}

func getString(root *dynbt.Value, keys ...string) string {
	value := getValue(root, keys...)
	if value == nil || value.TagType() != nbt.TagString {
		return ""
	}
	return value.String()
}

func nbtNumber(value *dynbt.Value) (int64, bool) {
	if value == nil {
		return 0, false
	}
	switch value.TagType() {
	case nbt.TagByte:
		return int64(value.Byte()), true
	case nbt.TagShort:
		return int64(value.Short()), true
	case nbt.TagInt:
		return int64(value.Int()), true
	case nbt.TagLong:
		return value.Long(), true
	default:
		return 0, false
	}
}

func nbtBoolean(value *dynbt.Value) bool {
	if value == nil {
		return false
	}
	if value.TagType() == nbt.TagCompound {
		return true
	}
	number, ok := nbtNumber(value)
	return ok && number != 0
}

func compactValue(value *dynbt.Value) string {
	if value == nil {
		return ""
	}
	if value.TagType() == nbt.TagString {
		return value.String()
	}
	if number, ok := nbtNumber(value); ok {
		return strconv.FormatInt(number, 10)
	}
	if value.TagType() == nbt.TagList {
		return listSummary(value)
	}
	if value.TagType() == nbt.TagCompound {
		return fmt.Sprintf("%d field(s)", value.Compound().Len())
	}
	return ""
}

func listSummary(value *dynbt.Value) string {
	if value == nil || value.TagType() != nbt.TagList {
		return ""
	}
	return fmt.Sprintf("%d item(s)", len(value.List()))
}

func plainMinecraftText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) != nil {
		return value
	}
	parts := make([]string, 0, 4)
	collectText(decoded, &parts)
	if len(parts) == 0 {
		return value
	}
	return strings.Join(parts, "")
}

func collectText(value any, output *[]string) {
	switch typed := value.(type) {
	case string:
		*output = append(*output, typed)
	case []any:
		for _, child := range typed {
			collectText(child, output)
		}
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			*output = append(*output, text)
		} else if translate, ok := typed["translate"].(string); ok {
			*output = append(*output, translate)
		}
		if extra, ok := typed["extra"]; ok {
			collectText(extra, output)
		}
	}
}

func firstText(values ...string) string {
	for _, value := range values {
		if text := plainMinecraftText(value); text != "" {
			return text
		}
	}
	return ""
}

func firstString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstValue(values ...*dynbt.Value) *dynbt.Value {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
