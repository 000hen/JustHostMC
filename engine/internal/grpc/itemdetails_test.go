package grpcsvc

import "testing"

func TestExtractItemDetailsModernComponents(t *testing.T) {
	raw := rawNBT(t, `{Slot:0B,id:"minecraft:diamond_sword",count:1,components:{"minecraft:custom_name":'{"text":"Blade"}',"minecraft:lore":['{"text":"Line one"}'],"minecraft:damage":12,"minecraft:max_damage":1561,"minecraft:enchantments":{levels:{"minecraft:sharpness":5}}}}`)
	details := extractItemDetails(raw)
	want := map[string]string{
		"Custom name":  "Blade",
		"Lore":         "Line one",
		"Damage":       "12",
		"Max damage":   "1561",
		"Enchantments": "minecraft:sharpness 5",
	}
	for _, detail := range details {
		if expected, ok := want[detail.Label]; ok {
			if detail.Value != expected {
				t.Errorf("%s = %q, want %q", detail.Label, detail.Value, expected)
			}
			delete(want, detail.Label)
		}
	}
	for label := range want {
		t.Errorf("missing detail %q", label)
	}
}

func TestExtractItemDetailsLegacyTag(t *testing.T) {
	raw := rawNBT(t, `{Slot:0B,id:"minecraft:diamond_sword",Count:1B,tag:{Damage:7,Unbreakable:1B,display:{Name:'{"text":"Legacy Blade"}',Lore:['{"text":"Old lore"}']},Enchantments:[{id:"minecraft:sharpness",lvl:3S}]}}`)
	details := extractItemDetails(raw)
	values := make(map[string]string)
	for _, detail := range details {
		values[detail.Label] = detail.Value
	}
	if values["Custom name"] != "Legacy Blade" || values["Lore"] != "Old lore" || values["Damage"] != "7" || values["Unbreakable"] != "Yes" {
		t.Fatalf("legacy details = %+v", values)
	}
}

func TestExtractItemDetailsPotionEffects(t *testing.T) {
	raw := rawNBT(t, `{Slot:0B,id:"minecraft:potion",count:1,components:{"minecraft:potion_contents":{potion:"minecraft:strong_healing",custom_effects:[{id:"minecraft:speed",amplifier:1,duration:1800}]}}}`)
	details := extractItemDetails(raw)
	values := make(map[string]string)
	for _, detail := range details {
		values[detail.Label] = detail.Value
	}
	if values["Potion"] != "minecraft:strong_healing" {
		t.Fatalf("potion = %q", values["Potion"])
	}
	if values["Effects"] != "Speed II (1:30)" {
		t.Fatalf("effects = %q", values["Effects"])
	}
}

func TestExtractItemDetailsIncludesRawComponentValues(t *testing.T) {
	raw := rawNBT(t, `{Slot:0B,id:"minecraft:stone",count:1,components:{"minecraft:custom_data":{owner:"Alex",level:3}}}`)
	details := extractItemDetails(raw)
	for _, detail := range details {
		if detail.Label == "Component · Custom Data" {
			if detail.Value != `{owner:Alex,level:3}` {
				t.Fatalf("component value = %q", detail.Value)
			}
			return
		}
	}
	t.Fatal("raw custom_data component detail was not included")
}
