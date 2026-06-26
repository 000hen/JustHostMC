package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Mirrors the live feed: legacy 3-part versions (21.1.66 -> MC 1.21.1) coexist
// with the current 4-part scheme (26.1.2.76 -> MC 26.1.2), including betas and a
// stray old release left in the metadata.
const neoMavenXML = `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning>
    <versions>
      <version>20.4.190</version>
      <version>21.0.167</version>
      <version>21.1.1</version>
      <version>21.1.66</version>
      <version>26.1.2.74</version>
      <version>26.1.2.76</version>
      <version>26.2.0.6-beta</version>
      <version>26.2.0.7-beta</version>
    </versions>
  </versioning>
</metadata>`

func newNeoForgeStub(t *testing.T) *NeoForge {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(neoMavenXML))
	}))
	t.Cleanup(srv.Close)
	return NewNeoForge(nil, WithNeoForgeMavenBase(srv.URL))
}

func TestNeoForgeVersionsMapToMC(t *testing.T) {
	n := newNeoForgeStub(t)
	versions, err := n.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 26.2.0.* -> 26.2, 26.1.2.* -> 26.1.2, 21.1.* -> 1.21.1, 21.0.* -> 1.21, 20.4.* -> 1.20.4
	want := []string{"26.2", "26.1.2", "1.21.1", "1.21", "1.20.4"}
	if len(versions) != len(want) {
		t.Fatalf("Versions = %v, want %v", versions, want)
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("Versions = %v, want %v", versions, want)
		}
	}
}

func TestNeoForgeResolvePicksHighestPatch(t *testing.T) {
	n := newNeoForgeStub(t)
	cases := map[string]string{
		"1.21.1": "21.1.66",
		"1.21":   "21.0.167",
		"26.1.2": "26.1.2.76",     // current 4-part scheme, highest build
		"26.2":   "26.2.0.7-beta", // MC patch 0 -> NeoForge 26.2.0.*
	}
	for mc, want := range cases {
		v, err := n.resolveVersion(context.Background(), mc)
		if err != nil {
			t.Fatalf("resolveVersion(%q): %v", mc, err)
		}
		if v != want {
			t.Errorf("resolveVersion(%q) = %q, want %q", mc, v, want)
		}
	}
}

func TestMcForNeoForge(t *testing.T) {
	cases := map[string]string{
		// legacy 3-part: A.B.<build> -> MC 1.A.B (B==0 -> 1.A)
		"21.1.66":  "1.21.1",
		"21.0.167": "1.21",
		"20.4.190": "1.20.4",
		// current 4-part: A.B.C.<build> -> MC A.B.C (C==0 -> A.B)
		"26.1.2.76":     "26.1.2",
		"26.1.1.5":      "26.1.1",
		"26.2.0.7-beta": "26.2",
		"bad":           "",
	}
	for in, want := range cases {
		if got := mcForNeoForge(in); got != want {
			t.Errorf("mcForNeoForge(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNeoForgePrefix(t *testing.T) {
	cases := map[string]string{
		// legacy MC 1.x -> NeoForge minor.patch.
		"1.21.1": "21.1.",
		"1.21":   "21.0.",
		// current MC scheme -> NeoForge major.minor.patch.
		"26.1.2": "26.1.2.",
		"26.1.1": "26.1.1.",
		"26.2":   "26.2.0.",
	}
	for mc, want := range cases {
		got, ok := neoForgePrefix(mc)
		if !ok || got != want {
			t.Errorf("neoForgePrefix(%q) = %q,%v, want %q,true", mc, got, ok, want)
		}
	}
}
