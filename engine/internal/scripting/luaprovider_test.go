package scripting

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/provider"
)

// TestBuiltinVanillaMeta loads the embedded vanilla script and checks its
// declared metadata + permissions parse correctly (no network).
func TestBuiltinVanillaMeta(t *testing.T) {
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	if err := LoadBuiltins(context.Background(), r); err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	e, ok := r.Get("vanilla")
	if !ok {
		t.Fatal("vanilla not registered")
	}
	if e.Meta.Name != "Vanilla" {
		t.Errorf("name = %q, want Vanilla", e.Meta.Name)
	}
	if e.Meta.ModLayout != "none" {
		t.Errorf("mod_layout = %q, want none", e.Meta.ModLayout)
	}
	if len(e.Meta.Permissions) != 1 ||
		e.Meta.Permissions[0].Kind != mcmanagerv1.PermissionKind_PERMISSION_NETWORK {
		t.Errorf("permissions = %+v, want [network]", e.Meta.Permissions)
	}
}

const jarBody = "PK\x03\x04 not really a jar"

// stubServer serves a Mojang-shaped manifest/detail/jar for the host test.
func stubServer(t *testing.T) *httptest.Server {
	t.Helper()
	sum := sha1.Sum([]byte(jarBody))
	sha := hex.EncodeToString(sum[:])
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/manifest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"versions":[{"id":"1.21","type":"release","url":%q}]}`, base+"/detail")
	})
	mux.HandleFunc("/detail", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"downloads":{"server":{"url":%q,"sha1":%q}},"javaVersion":{"majorVersion":21}}`, base+"/jar", sha)
	})
	mux.HandleFunc("/jar", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(jarBody))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func stubScript(url string) string {
	return fmt.Sprintf(`
meta = { id = "stub", name = "Stub", mod_layout = "none",
  permissions = { { kind = "network", reason = "test" } } }

function versions()
  local m = jhmc.http_json(%q)
  local out = {}
  for _, e in ipairs(m.versions) do out[#out + 1] = e.id end
  return out
end

function install(ctx)
  local m = jhmc.http_json(%q)
  local entry
  for _, e in ipairs(m.versions) do if e.id == ctx.version then entry = e end end
  if not entry then error("version not found: " .. ctx.version) end
  local d = jhmc.http_json(entry.url)
  jhmc.download(d.downloads.server.url, { dest = "server.jar", sha1 = d.downloads.server.sha1 })
  return { java_major = d.javaVersion.majorVersion, args = { "-jar", "server.jar", "nogui" } }
end
`, url+"/manifest", url+"/manifest")
}

func TestHostVersionsAndInstall(t *testing.T) {
	srv := stubServer(t)
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	e, err := r.AddSource(context.Background(), stubScript(srv.URL), true) // builtin → network auto-granted
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	vers, err := e.Provider.Versions(context.Background())
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(vers) != 1 || vers[0] != "1.21" {
		t.Fatalf("Versions = %v, want [1.21]", vers)
	}

	dir := t.TempDir()
	var steps []string
	spec, err := e.Provider.Install(context.Background(), dir, "1.21", func(p provider.Progress) {
		if p.Step != "" {
			steps = append(steps, p.Step)
		}
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if spec.JavaMajor != 21 {
		t.Errorf("JavaMajor = %d, want 21", spec.JavaMajor)
	}
	if strings.Join(spec.Args, " ") != "-jar server.jar nogui" {
		t.Errorf("Args = %v", spec.Args)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "server.jar")); err != nil || string(b) != jarBody {
		t.Errorf("server.jar not downloaded correctly: %v", err)
	}
}

// TestInstallLaunchSpecReadsMcVersionAndLoader verifies a provider that resolves
// a concrete version/loader inside install() surfaces them on the LaunchSpec —
// the hook a modpack provider uses to override an opaque "packId/versionId".
func TestInstallLaunchSpecReadsMcVersionAndLoader(t *testing.T) {
	script := `
meta = { id = "resolv", name = "Resolv", mod_layout = "mods" }
function versions() return {} end
function install(ctx)
  return { java_major = 21, args = { "-jar", "server.jar", "nogui" },
           mc_version = "1.20.1", loader = "forge" }
end
`
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	e, err := r.AddSource(context.Background(), script, true)
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	spec, err := e.Provider.Install(context.Background(), t.TempDir(), "pack/42", nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if spec.McVersion != "1.20.1" {
		t.Errorf("McVersion = %q, want 1.20.1", spec.McVersion)
	}
	if spec.Loader != "forge" {
		t.Errorf("Loader = %q, want forge", spec.Loader)
	}
}

// TestInstallLaunchSpecReadsPackVersion verifies a modpack provider can persist
// its opaque "packId/versionId" through the launch spec.
func TestInstallLaunchSpecReadsPackVersion(t *testing.T) {
	script := `
meta = { id = "packy", name = "Packy", mod_layout = "mods" }
function versions() return {} end
function install(ctx)
  return { java_major = 21, args = { "-jar", "server.jar", "nogui" },
           mc_version = "1.20.1", loader = "neoforge",
           pack_version = tostring(ctx.version) }
end
`
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	e, err := r.AddSource(context.Background(), script, true)
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	spec, err := e.Provider.Install(context.Background(), t.TempDir(), "95/12695", nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if spec.PackVersion != "95/12695" {
		t.Errorf("PackVersion = %q, want 95/12695", spec.PackVersion)
	}
}

// TestUngrantedNetworkDenied proves a non-builtin script with no grants cannot
// reach the network.
func TestUngrantedNetworkDenied(t *testing.T) {
	srv := stubServer(t)
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	e, err := r.AddSource(context.Background(), stubScript(srv.URL), false) // not builtin, no grants
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if _, err := e.Provider.Versions(context.Background()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Versions err = %v, want ErrPermissionDenied", err)
	}
}

// TestVersionNotFoundMapped checks the script idiom maps to the provider sentinel.
func TestVersionNotFoundMapped(t *testing.T) {
	srv := stubServer(t)
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	e, _ := r.AddSource(context.Background(), stubScript(srv.URL), true)
	_, err := e.Provider.Install(context.Background(), t.TempDir(), "9.9.9", nil)
	if !errors.Is(err, provider.ErrVersionNotFound) {
		t.Fatalf("Install err = %v, want ErrVersionNotFound", err)
	}
}
