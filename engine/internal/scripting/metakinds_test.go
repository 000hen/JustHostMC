package scripting

import "testing"

func TestParseMetaKinds(t *testing.T) {
	m, err := metaFrom(t, `meta = { id = "x", kinds = { "modpack" } }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Kinds) != 1 || m.Kinds[0] != "modpack" {
		t.Fatalf("kinds = %v, want [modpack]", m.Kinds)
	}
}

func TestParseMetaKindsNormalizesAndAllowsAll(t *testing.T) {
	m, err := metaFrom(t, `meta = { id = "x", kinds = { "MOD", " Plugin ", "modpack" } }`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mod", "plugin", "modpack"}
	if len(m.Kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", m.Kinds, want)
	}
	for i, k := range want {
		if m.Kinds[i] != k {
			t.Fatalf("kinds[%d] = %q, want %q", i, m.Kinds[i], k)
		}
	}
}

func TestParseMetaKindsRejectsUnknown(t *testing.T) {
	if _, err := metaFrom(t, `meta = { id = "x", kinds = { "widget" } }`); err == nil {
		t.Fatal("expected an error for an unknown kind")
	}
	if _, err := metaFrom(t, `meta = { id = "x", kinds = { 3 } }`); err == nil {
		t.Fatal("expected an error for a non-string kind")
	}
}

func TestParseMetaKindsAbsentIsEmpty(t *testing.T) {
	// The default {"mod","plugin"} is applied by the shop service, not parseMeta,
	// so an absent meta.kinds parses to an empty slice.
	m, err := metaFrom(t, `meta = { id = "x" }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Kinds) != 0 {
		t.Fatalf("kinds = %v, want empty", m.Kinds)
	}
}

func TestParseMetaHidden(t *testing.T) {
	m, err := metaFrom(t, `meta = { id = "x", hidden = true }`)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Hidden {
		t.Fatal("hidden should be true")
	}

	m2, err := metaFrom(t, `meta = { id = "x" }`)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Hidden {
		t.Fatal("hidden should default to false")
	}
}
