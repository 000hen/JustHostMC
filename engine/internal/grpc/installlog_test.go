package grpcsvc

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc"
)

// installStubProvider emits scripted progress then returns a fixed result.
type installStubProvider struct {
	emit []provider.Progress
	spec provider.LaunchSpec
	err  error
}

func (p *installStubProvider) Versions(context.Context) ([]string, error) { return nil, nil }
func (p *installStubProvider) Install(_ context.Context, _, _ string, report func(provider.Progress)) (provider.LaunchSpec, error) {
	for _, pr := range p.emit {
		if report != nil {
			report(pr)
		}
	}
	return p.spec, p.err
}

// fakeInstallStream is a no-op InstallProgress stream for driving Create.
type fakeInstallStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeInstallStream) Send(*mcmanagerv1.InstallProgress) error { return nil }
func (f *fakeInstallStream) Context() context.Context {
	if f.ctx != nil {
		return f.ctx
	}
	return context.Background()
}

func readAnyInstallLog(t *testing.T, logsRoot string) string {
	t.Helper()
	var found string
	_ = filepath.WalkDir(logsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), "install-") {
			b, _ := os.ReadFile(path)
			found = string(b)
		}
		return nil
	})
	if found == "" {
		t.Fatalf("no install log found under %s", logsRoot)
	}
	return found
}

func TestCreatePersistsInstallLogOnFailure(t *testing.T) {
	paths := appdata.New(t.TempDir())
	prov := &installStubProvider{
		emit: []provider.Progress{
			{Step: "install.vanilla.download"},
			{LogLine: "downloading server.jar"},
		},
		err: provider.ErrVersionNotFound,
	}
	svc := NewServerService(ServerServiceConfig{
		Store:     store.NewMemory(),
		Providers: testRegistry("vanilla", "none", prov),
		Paths:     paths,
		Backend:   &fakeBackend{},
	})

	err := svc.Create(
		&mcmanagerv1.CreateServerRequest{Name: "x", ProviderId: "vanilla", McVersion: "1.20.1"},
		&fakeInstallStream{},
	)
	if err == nil {
		t.Fatal("expected install to fail")
	}

	content := readAnyInstallLog(t, paths.LogsRoot())
	if !strings.Contains(content, "downloading server.jar") {
		t.Errorf("install log missing raw output:\n%s", content)
	}
	if !strings.Contains(content, "[error] install:") {
		t.Errorf("install log missing error cause:\n%s", content)
	}
}
