package scripting

import (
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

func TestConfigStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cs := NewConfigStore(path)

	if v := cs.Values("s1"); len(v) != 0 {
		t.Fatalf("empty store returned %v", v)
	}
	if err := cs.Set("s1", "region", "eu"); err != nil {
		t.Fatal(err)
	}
	if err := cs.Set("s1", "token", "abc"); err != nil {
		t.Fatal(err)
	}

	// Reload from disk to prove persistence.
	got := NewConfigStore(path).Values("s1")
	if got["region"] != "eu" || got["token"] != "abc" {
		t.Fatalf("reloaded values = %v", got)
	}

	// Values returns a copy: mutating it must not affect the store.
	got["region"] = "mutated"
	if cs.Values("s1")["region"] != "eu" {
		t.Fatal("Values did not return a copy")
	}
}

func TestConfigStoreClearAndForget(t *testing.T) {
	cs := NewConfigStore(filepath.Join(t.TempDir(), "config.json"))
	_ = cs.Set("s1", "a", "1")
	_ = cs.Set("s1", "b", "2")

	// Empty value clears one key.
	if err := cs.Set("s1", "a", ""); err != nil {
		t.Fatal(err)
	}
	got := cs.Values("s1")
	if _, ok := got["a"]; ok || got["b"] != "2" {
		t.Fatalf("after clearing a: %v", got)
	}

	// Forget drops everything for the id.
	if err := cs.Forget("s1"); err != nil {
		t.Fatal(err)
	}
	if len(cs.Values("s1")) != 0 {
		t.Fatal("Forget left data")
	}
	// Clearing a key on an unknown id is a no-op, not an error.
	if err := cs.Set("gone", "x", ""); err != nil {
		t.Fatal(err)
	}
}

func TestEffectiveConfig(t *testing.T) {
	opts := []ConfigOption{
		{Key: "region", Type: mcmanagerv1.ConfigOptionType_CONFIG_OPTION_STRING, Default: "us"},
		{Key: "count", Type: mcmanagerv1.ConfigOptionType_CONFIG_OPTION_NUMBER, Default: "3"},
		{Key: "empty", Type: mcmanagerv1.ConfigOptionType_CONFIG_OPTION_STRING}, // no default
	}
	// Declared defaults only; an option without a default does not appear.
	eff := EffectiveConfig(opts, nil)
	if eff["region"] != "us" || eff["count"] != "3" {
		t.Fatalf("defaults: %v", eff)
	}
	if _, ok := eff["empty"]; ok {
		t.Fatal("option without a default must not appear")
	}
	// Stored overrides win over declared defaults.
	eff = EffectiveConfig(opts, map[string]string{"region": "eu", "empty": "x"})
	if eff["region"] != "eu" || eff["count"] != "3" || eff["empty"] != "x" {
		t.Fatalf("override: %v", eff)
	}
}
