package scripting

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/000hen/justhostmc/engine/internal/provider"
)

// stubRoundTripper serves both official NeoForge Maven feeds and records the
// requested paths so resolution can be tested without the network.
type stubRoundTripper struct{ paths []string }

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.paths = append(s.paths, req.URL.Path)
	body := "installer"
	switch req.URL.Path {
	case "/releases/net/neoforged/neoforge/maven-metadata.xml":
		body = neoForgeMetaXML
	case "/releases/net/neoforged/forge/maven-metadata.xml":
		body = legacyNeoForgeMetaXML
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

// neoForgeMetaXML mixes the version shapes NeoForge's maven now publishes:
// 3-part legacy (1.x), 3-part current that maps to a 2-segment MC ("21.0.x" ->
// "1.21"), 4-part MC-2026, a 4-part build whose patch is 0 (maps to a 2-segment
// MC "26.2"), and an alpha/snapshot qualifier containing a dot. Qualifier dots
// do not change the first four coordinate components and must not hide MC 26.1.
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

// NeoForge 1.20.1 predates the net.neoforged:neoforge artifact. Its builds are
// in net.neoforged:forge and encode the full Minecraft version before a dash.
// The feed also contains one malformed historical coordinate that must be ignored.
const legacyNeoForgeMetaXML = `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <versioning><versions>
    <version>1.20.1-47.1.105</version>
    <version>47.1.107</version>
    <version>1.20.1-47.1.106</version>
  </versions></versioning>
</metadata>`

// TestNeoForgeVersionsParsing runs the real neoforge.lua versions() against the
// canned metadata above. It regression-guards the gopher-lua crash where a
// string.match no-match in the final argument position passes zero values to
// tonumber() ("value expected") — triggered by 2-segment MC versions like
// "26.2"/"1.21" whose parse_mc has no patch part.
func TestNeoForgeVersionsParsing(t *testing.T) {
	transport := &stubRoundTripper{}
	client := &http.Client{Transport: transport}
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

	want := []string{"26.2", "26.1.2", "26.1", "1.21.1", "1.21", "1.20.2", "1.20.1"}
	if len(vers) != len(want) {
		t.Fatalf("Versions = %v, want %v", vers, want)
	}
	for i, w := range want {
		if vers[i] != w {
			t.Fatalf("Versions = %v, want %v", vers, want)
		}
	}
}

// TestNeoForgeLegacyInstallerResolution verifies that 1.20.1 uses the legacy
// artifact name and highest numeric build. The fake Java path intentionally
// fails after the installer download; the requested URL is the assertion target.
func TestNeoForgeLegacyInstallerResolution(t *testing.T) {
	transport := &stubRoundTripper{}
	client := &http.Client{Transport: transport}
	missingJava := filepath.Join(t.TempDir(), "missing-java")
	jre := func(context.Context, int, func(provider.Progress)) (string, error) {
		return missingJava, nil
	}
	r := NewRegistry(NewHost(client, jre, nil), nil)
	if err := LoadBuiltins(r); err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	e, ok := r.Get("neoforge")
	if !ok {
		t.Fatal("neoforge provider not registered")
	}

	if _, err := e.Provider.Install(context.Background(), t.TempDir(), "1.20.1", nil); err == nil {
		t.Fatal("Install unexpectedly succeeded with a missing Java executable")
	}

	want := "/releases/net/neoforged/forge/1.20.1-47.1.106/forge-1.20.1-47.1.106-installer.jar"
	for _, path := range transport.paths {
		if path == want {
			return
		}
	}
	t.Fatalf("installer URL %q was not requested; paths = %v", want, transport.paths)
}
