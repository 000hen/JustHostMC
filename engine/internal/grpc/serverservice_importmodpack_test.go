package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// importStubProvider records the dir/version it was installed with and whether
// the modpack file was staged into the server dir before install ran.
type importStubProvider struct {
	spec       provider.LaunchSpec
	stagedSeen bool
	gotVersion string
}

func (p *importStubProvider) Versions(context.Context) ([]string, error) { return nil, nil }
func (p *importStubProvider) Install(_ context.Context, dir, version string, _ func(provider.Progress)) (provider.LaunchSpec, error) {
	p.gotVersion = version
	if _, err := os.Stat(filepath.Join(dir, ".jhmc", "import.zip")); err == nil {
		p.stagedSeen = true
	}
	return p.spec, nil
}

func importTestService(t *testing.T, prov provider.Provider) (*ServerService, *store.Memory) {
	t.Helper()
	paths := appdata.New(t.TempDir())
	cacheJava(t, paths.JRECache(), 21)
	st := store.NewMemory()
	svc := NewServerService(ServerServiceConfig{
		Store:     st,
		Providers: testRegistry("import", "mods", prov),
		Paths:     paths,
		JRE:       jre.NewManager(paths.JRECache()),
	})
	return svc, st
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportModpackStagesAndCreates(t *testing.T) {
	prov := &importStubProvider{spec: provider.LaunchSpec{
		JavaMajor: 21, McVersion: "1.20.1", Loader: "forge",
		Args: []string{"@run"}, PackVersion: "import/1.0",
	}}
	svc, st := importTestService(t, prov)
	src := writeTempFile(t, "pack.zip", "PK fake zip")

	if err := svc.ImportModpack(&mcmanagerv1.ImportModpackRequest{
		Name: "My Import", SrcPath: src, MemoryMb: 4096,
	}, &fakeInstallStream{}); err != nil {
		t.Fatalf("ImportModpack: %v", err)
	}
	if !prov.stagedSeen {
		t.Error("staged .jhmc/import.zip missing when the provider ran")
	}
	if prov.gotVersion != "import/local" {
		t.Errorf("provider version = %q, want import/local", prov.gotVersion)
	}
	servers := st.List()
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(servers))
	}
	rec := servers[0]
	if rec.Name != "My Import" || rec.ProviderID != "import" || rec.ProviderVersion != "import/1.0" {
		t.Errorf("record = %+v", rec)
	}
	if rec.Status != mcmanagerv1.ServerStatus_STOPPED {
		t.Errorf("status = %v, want STOPPED", rec.Status)
	}
	if rec.McVersion != "1.20.1" || rec.Loader != "forge" {
		t.Errorf("record mc/loader = %q/%q", rec.McVersion, rec.Loader)
	}
}

func TestImportModpackValidation(t *testing.T) {
	svc, st := importTestService(t, &importStubProvider{})
	good := writeTempFile(t, "ok.zip", "z")

	cases := []struct {
		name string
		req  *mcmanagerv1.ImportModpackRequest
	}{
		{"empty name", &mcmanagerv1.ImportModpackRequest{Name: "  ", SrcPath: good}},
		{"relative path", &mcmanagerv1.ImportModpackRequest{Name: "n", SrcPath: "pack.zip"}},
		{"bad extension", &mcmanagerv1.ImportModpackRequest{Name: "n", SrcPath: writeTempFile(t, "pack.rar", "z")}},
		{"missing file", &mcmanagerv1.ImportModpackRequest{Name: "n", SrcPath: filepath.Join(t.TempDir(), "nope.zip")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := svc.ImportModpack(c.req, &fakeInstallStream{})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
			}
		})
	}
	if n := len(st.List()); n != 0 {
		t.Errorf("no server should be created on validation failure, got %d", n)
	}
}
