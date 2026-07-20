package scripting

import (
	"context"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// metaFrom parses just the meta table of a script source (no functions needed).
func metaFrom(t *testing.T, src string) (Meta, error) {
	t.Helper()
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	L, err := inv.prepare(src)
	if err != nil {
		return Meta{}, err
	}
	defer L.Close()
	return parseMeta(L)
}

func TestParseMetaConfigValid(t *testing.T) {
	m, err := metaFrom(t, `
meta = { id = "x", config = {
  { key = "region", type = "string", name = "Region", description = "d", default = "us", required = true },
  { key = "count", type = "number", default = "5" },
  { key = "flag", type = "boolean", default = "true" },
  { key = "token", type = "secret" },
  { key = "plain" },
} }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Config) != 5 {
		t.Fatalf("config len = %d, want 5", len(m.Config))
	}
	r := m.Config[0]
	if r.Key != "region" || r.Type != mcmanagerv1.ConfigOptionType_CONFIG_OPTION_STRING ||
		r.Name != "Region" || r.Description != "d" || r.Default != "us" || !r.Required {
		t.Fatalf("region option: %+v", r)
	}
	if m.Config[1].Type != mcmanagerv1.ConfigOptionType_CONFIG_OPTION_NUMBER ||
		m.Config[2].Type != mcmanagerv1.ConfigOptionType_CONFIG_OPTION_BOOLEAN ||
		m.Config[3].Type != mcmanagerv1.ConfigOptionType_CONFIG_OPTION_SECRET {
		t.Fatalf("types: %+v", m.Config)
	}
	// A missing type defaults to string.
	if m.Config[4].Type != mcmanagerv1.ConfigOptionType_CONFIG_OPTION_STRING {
		t.Fatalf("plain option type = %v", m.Config[4].Type)
	}
}

func TestParseMetaConfigRejectsBad(t *testing.T) {
	cases := map[string]string{
		"unknown type": `meta = { id = "x", config = { { key = "k", type = "colour" } } }`,
		"dup key":      `meta = { id = "x", config = { { key = "k" }, { key = "k" } } }`,
		"missing key":  `meta = { id = "x", config = { { type = "string" } } }`,
		"bad key char": `meta = { id = "x", config = { { key = "a.b" } } }`,
		"bad number":   `meta = { id = "x", config = { { key = "k", type = "number", default = "abc" } } }`,
		"bad boolean":  `meta = { id = "x", config = { { key = "k", type = "boolean", default = "maybe" } } }`,
		"not a table":  `meta = { id = "x", config = { "nope" } }`,
	}
	for name, src := range cases {
		if _, err := metaFrom(t, src); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}
