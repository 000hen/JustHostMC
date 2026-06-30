package scripting

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubRoundTripper serves a single canned body for every request, so a built-in
// script that hits a hard-coded upstream URL can be tested without the network.
type stubRoundTripper struct{ body string }

func (s stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}, nil
}

// neoForgeMetaXML mixes the version shapes NeoForge's maven now publishes:
// 3-part legacy (1.x), 3-part current that maps to a 2-segment MC ("21.0.x" ->
// "1.21"), 4-part MC-2026, a 4-part build whose patch is 0 (maps to a 2-segment
// MC "26.2"), and 5-part alpha/snapshot builds (which have no clean MC mapping
// and must be skipped, not crash the parser).
const neoForgeMetaXML = `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning><versions>
    <version>20.2.12-beta</version>
    <version>21.0.5</version>
    <version>21.1.234</version>
    <version>26.1.2.76</version>
    <version>26.2.0.6-beta</version>
    <version>26.1.0.0-alpha.1+snapshot-1</version>
  </versions></versioning>
</metadata>`

// TestNeoForgeVersionsParsing runs the real neoforge.lua versions() against the
// canned metadata above. It regression-guards the gopher-lua crash where a
// string.match no-match in the final argument position passes zero values to
// tonumber() ("value expected") — triggered by 2-segment MC versions like
// "26.2"/"1.21" whose parse_mc has no patch part.
func TestNeoForgeVersionsParsing(t *testing.T) {
	client := &http.Client{Transport: stubRoundTripper{neoForgeMetaXML}}
	r := NewRegistry(NewHost(client, nil, nil), nil)
	if err := LoadBuiltins(r); err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	e, ok := r.Get("neoforge")
	if !ok {
		t.Fatal("neoforge provider not registered")
	}

	vers, err := e.Provider.Versions(context.Background())
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	want := []string{"26.2", "26.1.2", "1.21.1", "1.21", "1.20.2"}
	if len(vers) != len(want) {
		t.Fatalf("Versions = %v, want %v", vers, want)
	}
	for i, w := range want {
		if vers[i] != w {
			t.Fatalf("Versions = %v, want %v", vers, want)
		}
	}
}
