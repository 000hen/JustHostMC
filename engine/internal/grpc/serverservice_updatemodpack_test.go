package grpcsvc

import (
	"context"
	"errors"
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

// updateStubProvider is an installStubProvider that also implements
// provider.Updater, recording the versions it was called with.
type updateStubProvider struct {
	installStubProvider
	gotVersion, gotOld string
	updateSpec         provider.LaunchSpec
	updateErr          error
}

func (p *updateStubProvider) Update(_ context.Context, _, version, oldVersion string, report func(provider.Progress)) (provider.LaunchSpec, error) {
	p.gotVersion, p.gotOld = version, oldVersion
	if report != nil {
		report(provider.Progress{Step: "shop.install.downloading", Fraction: 0.5})
	}
	return p.updateSpec, p.updateErr
}

func modpackTestService(t *testing.T, prov provider.Provider, status mcmanagerv1.ServerStatus, providerVersion string) (*ServerService, *store.Memory, appdata.Paths) {
	t.Helper()
	paths := appdata.New(t.TempDir())
	if err := os.MkdirAll(paths.ServerDir("s1"), 0o755); err != nil {
		t.Fatal(err)
	}
	cacheJava(t, paths.JRECache(), 21)
	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "Pack", ProviderID: "ftb", ModLayout: "mods",
		McVersion: "1.21.1", Loader: "neoforge", ProviderVersion: providerVersion,
		MemoryMB: 4096, Port: 25565, Status: status,
		JavaMajor: 21, LaunchArgs: []string{"-jar", "old-run.jar"},
	})
	svc := NewServerService(ServerServiceConfig{
		Store:     st,
		Providers: testRegistry("ftb", "mods", prov),
		Paths:     paths,
		JRE:       jre.NewManager(paths.JRECache()),
	})
	return svc, st, paths
}

func TestUpdateModpackHappyPath(t *testing.T) {
	prov := &updateStubProvider{updateSpec: provider.LaunchSpec{
		JavaMajor: 21, McVersion: "1.21.4", Loader: "neoforge",
		PackVersion: "95/300",
	}}
	svc, st, _ := modpackTestService(t, prov, mcmanagerv1.ServerStatus_STOPPED, "95/100")

	err := svc.UpdateModpack(&mcmanagerv1.UpdateModpackRequest{Id: "s1", Version: "95/300"},
		&fakeInstallStream{})
	if err != nil {
		t.Fatalf("UpdateModpack: %v", err)
	}
	if prov.gotVersion != "95/300" || prov.gotOld != "95/100" {
		t.Errorf("provider got (%q, %q), want (95/300, 95/100)", prov.gotVersion, prov.gotOld)
	}
	rec, _ := st.Get("s1")
	if rec.ProviderVersion != "95/300" || rec.McVersion != "1.21.4" {
		t.Errorf("record = %+v, want new pack + MC version", rec)
	}
	if rec.Status != mcmanagerv1.ServerStatus_STOPPED {
		t.Errorf("status = %v, want STOPPED", rec.Status)
	}
	// spec.Args empty → existing launch args kept.
	if len(rec.LaunchArgs) != 2 || rec.LaunchArgs[1] != "old-run.jar" {
		t.Errorf("LaunchArgs = %v, want preserved", rec.LaunchArgs)
	}
}

func TestUpdateModpackReplacesArgsWhenProvided(t *testing.T) {
	prov := &updateStubProvider{updateSpec: provider.LaunchSpec{
		JavaMajor: 21, Args: []string{"@libraries/run.args"}, McVersion: "1.21.4",
		PackVersion: "95/300",
	}}
	svc, st, _ := modpackTestService(t, prov, mcmanagerv1.ServerStatus_STOPPED, "95/100")
	if err := svc.UpdateModpack(&mcmanagerv1.UpdateModpackRequest{Id: "s1", Version: "95/300"},
		&fakeInstallStream{}); err != nil {
		t.Fatalf("UpdateModpack: %v", err)
	}
	rec, _ := st.Get("s1")
	if len(rec.LaunchArgs) != 1 || rec.LaunchArgs[0] != "@libraries/run.args" {
		t.Errorf("LaunchArgs = %v, want replaced", rec.LaunchArgs)
	}
}

func TestUpdateModpackRejectsRunning(t *testing.T) {
	prov := &updateStubProvider{}
	svc, _, _ := modpackTestService(t, prov, mcmanagerv1.ServerStatus_RUNNING, "95/100")
	err := svc.UpdateModpack(&mcmanagerv1.UpdateModpackRequest{Id: "s1", Version: "95/300"},
		&fakeInstallStream{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", status.Code(err))
	}
}

func TestUpdateModpackRejectsNonModpackServer(t *testing.T) {
	prov := &updateStubProvider{}
	svc, _, _ := modpackTestService(t, prov, mcmanagerv1.ServerStatus_STOPPED, "")
	err := svc.UpdateModpack(&mcmanagerv1.UpdateModpackRequest{Id: "s1", Version: "95/300"},
		&fakeInstallStream{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", status.Code(err))
	}
}

// TestUpdateModpackUnsupportedIsUnimplemented proves a provider that reports
// ErrUpdateUnsupported (e.g. an imported pack, whose script has no update())
// surfaces as gRPC Unimplemented, not Internal, and leaves the server STOPPED.
func TestUpdateModpackUnsupportedIsUnimplemented(t *testing.T) {
	prov := &updateStubProvider{updateErr: provider.ErrUpdateUnsupported}
	svc, st, _ := modpackTestService(t, prov, mcmanagerv1.ServerStatus_STOPPED, "import/local")

	err := svc.UpdateModpack(&mcmanagerv1.UpdateModpackRequest{Id: "s1", Version: "import/local"},
		&fakeInstallStream{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("code = %v, want Unimplemented", status.Code(err))
	}
	rec, _ := st.Get("s1")
	if rec.Status != mcmanagerv1.ServerStatus_STOPPED {
		t.Errorf("status = %v, want restored STOPPED", rec.Status)
	}
}

// TestUpdateModpackFailureLeavesInstallIntact proves an update error restores
// STOPPED, keeps the record and the server dir — unlike Create's wipe.
func TestUpdateModpackFailureLeavesInstallIntact(t *testing.T) {
	prov := &updateStubProvider{updateErr: errors.New("cdn exploded")}
	svc, st, paths := modpackTestService(t, prov, mcmanagerv1.ServerStatus_STOPPED, "95/100")
	marker := filepath.Join(paths.ServerDir("s1"), "world-data.txt")
	if err := os.WriteFile(marker, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := svc.UpdateModpack(&mcmanagerv1.UpdateModpackRequest{Id: "s1", Version: "95/300"},
		&fakeInstallStream{})
	if err == nil {
		t.Fatal("UpdateModpack succeeded, want error")
	}
	rec, ok := st.Get("s1")
	if !ok {
		t.Fatal("record deleted on failure")
	}
	if rec.Status != mcmanagerv1.ServerStatus_STOPPED {
		t.Errorf("status = %v, want restored STOPPED", rec.Status)
	}
	if rec.ProviderVersion != "95/100" {
		t.Errorf("ProviderVersion = %q, want unchanged 95/100", rec.ProviderVersion)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("server dir wiped on failure: %v", err)
	}
}
