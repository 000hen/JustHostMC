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
)

func TestForgeServerLifecycleEndToEnd(t *testing.T) {
	if os.Getenv("JHMC_INTEGRATION") != "1" {
		t.Skip("set JHMC_INTEGRATION=1 to run (downloads Forge installer + libraries + JRE, runs the installer, then the server)")
	}

	const (
		version = "1.20.1"
		port    = "25602"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	paths := appdata.New(t.TempDir())
	dir := paths.ServerDir("forge-itest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	jreMgr := jre.NewManager(paths.JRECache())
	forge := provider.NewForge(jreMgr.EnsureJRE)

	// Install runs the Forge installer's --installServer, streaming its output.
	spec, err := forge.Install(ctx, dir, version, func(p provider.Progress) {
		if p.Step != "" {
			t.Logf("step: %s", p.Step)
		}
		if p.LogLine != "" {
			t.Logf("installer: %s", p.LogLine)
		}
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	t.Logf("forge launch args: %v (java %d)", spec.Args, spec.JavaMajor)

	javaPath, err := jreMgr.EnsureJRE(ctx, spec.JavaMajor, nil)
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
		ID:       "forge-itest",
		Dir:      dir,
		JavaPath: javaPath,
		Args:     append([]string{"-Xmx2048M"}, spec.Args...),
		MemoryMB: 3072,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	ready := false
	deadline := time.After(8 * time.Minute)
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

	if err := backend.Stop(ctx, "forge-itest", true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-inst.Done():
	case <-time.After(90 * time.Second):
		t.Fatal("server did not stop")
	}
}
