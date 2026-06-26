//go:build windows

// Package e2e holds heavy, network- and Java-dependent acceptance tests. They
// are skipped unless JHMC_INTEGRATION=1 (they download ~100 MB and run a real
// Minecraft server), satisfying the PROMPT §13 "tiny vanilla server" test.
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

func TestVanillaServerLifecycleEndToEnd(t *testing.T) {
	if os.Getenv("JHMC_INTEGRATION") != "1" {
		t.Skip("set JHMC_INTEGRATION=1 to run (downloads ~100 MB and runs a real server)")
	}

	const (
		version = "1.21.1"
		port    = 25599
	)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	paths := appdata.New(t.TempDir())
	dir := paths.ServerDir("itest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Download the vanilla server jar.
	spec, err := provider.NewVanilla().Install(ctx, dir, version, func(p provider.Progress) {
		if p.Step != "" {
			t.Logf("install step: %s (%.0f%%)", p.Step, p.Fraction*100)
		}
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	t.Logf("launch spec: java %d, args %v", spec.JavaMajor, spec.Args)

	// 2. Download the matching JRE on demand (no Java pre-installed assumption).
	javaPath, err := jre.NewManager(paths.JRECache()).EnsureJRE(ctx, spec.JavaMajor, func(p provider.Progress) {
		if p.Step != "" {
			t.Logf("jre step: %s", p.Step)
		}
	})
	if err != nil {
		t.Fatalf("EnsureJRE: %v", err)
	}

	// 3. Accept EULA and pin the port.
	if err := os.WriteFile(filepath.Join(dir, "eula.txt"), []byte("eula=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("server-port=25599\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Start it under a job object with a memory cap.
	backend := isolation.NewJobObjectBackend()
	inst, err := backend.Start(ctx, isolation.InstanceSpec{
		ID:       "itest",
		Dir:      dir,
		JavaPath: javaPath,
		Args:     append([]string{"-Xmx1024M"}, spec.Args...),
		MemoryMB: 2048,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 5. Wait for the server to report readiness.
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

	// 6. Stop it gracefully and confirm the process tree is gone.
	if err := backend.Stop(ctx, "itest", true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-inst.Done():
	case <-time.After(90 * time.Second):
		t.Fatal("server did not stop")
	}
	if inst.Running() {
		t.Error("server still running after stop")
	}
}
