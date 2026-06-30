//go:build windows

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/000hen/justhostmc/engine/internal/scripting"
)

// paperProvider returns the built-in paper Lua provider for integration tests
// (the Go provider.NewPaper() was replaced by the embedded script).
func paperProvider(t *testing.T) provider.Provider {
	t.Helper()
	reg := scripting.NewRegistry(scripting.NewHost(nil, nil, nil), nil)
	if err := scripting.LoadBuiltins(reg); err != nil {
		t.Fatalf("load builtin providers: %v", err)
	}
	e, ok := reg.Get("paper")
	if !ok {
		t.Fatal("paper provider not registered")
	}
	return e.Provider
}

func TestPaperServerLifecycleEndToEnd(t *testing.T) {
	if os.Getenv("JHMC_INTEGRATION") != "1" {
		t.Skip("set JHMC_INTEGRATION=1 to run (downloads Paper + JRE and runs a real server)")
	}

	const (
		version = "1.21.1"
		port    = "25601"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	paths := appdata.New(t.TempDir())
	dir := paths.ServerDir("paper-itest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	spec, err := paperProvider(t).Install(ctx, dir, version, func(p provider.Progress) {
		if p.Step != "" {
			t.Logf("install step: %s", p.Step)
		}
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	javaPath, err := jre.NewManager(paths.JRECache()).EnsureJRE(ctx, spec.JavaMajor, nil)
	if err != nil {
		t.Fatalf("EnsureJRE: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "eula.txt"), []byte("eula=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("server-port="+port+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := isolation.NewJobObjectBackend()
	inst, err := backend.Start(ctx, isolation.InstanceSpec{
		ID:       "paper-itest",
		Dir:      dir,
		JavaPath: javaPath,
		Args:     append([]string{"-Xmx1024M"}, spec.Args...),
		MemoryMB: 2048,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	ready := false
	deadline := time.After(6 * time.Minute)
	for !ready {
		select {
		case line, ok := <-inst.Output():
			if !ok {
				t.Fatal("server output closed before 'Done ('")
			}
			t.Log(line)
			if strings.Contains(line, "Done (") {
				ready = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for server readiness")
		}
	}

	if err := backend.Stop(ctx, "paper-itest", true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-inst.Done():
	case <-time.After(90 * time.Second):
		t.Fatal("server did not stop")
	}
}
