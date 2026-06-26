package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/settings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newSettingsService(t *testing.T) (*SettingsService, string) {
	t.Helper()
	base := t.TempDir()
	logsRoot := filepath.Join(base, "logs")
	st := settings.NewStore(filepath.Join(base, "settings.json"))
	return NewSettingsService(SettingsServiceConfig{Store: st, LogsRoot: logsRoot}), logsRoot
}

func TestSettingsServiceGetReturnsDefaults(t *testing.T) {
	svc, _ := newSettingsService(t)
	got, err := svc.GetLogRetention(context.Background(), &mcmanagerv1.Empty{})
	if err != nil {
		t.Fatalf("GetLogRetention: %v", err)
	}
	def := settings.Defaults()
	if got.KeepDays != int32(def.KeepLogDays) || got.MaxTotalBytes != def.MaxLogTotalBytes {
		t.Errorf("got %+v, want defaults %+v", got, def)
	}
}

func TestSettingsServiceSetThenGet(t *testing.T) {
	svc, _ := newSettingsService(t)
	ctx := context.Background()
	if _, err := svc.SetLogRetention(ctx, &mcmanagerv1.LogRetention{KeepDays: 30, MaxTotalBytes: 1 << 30}); err != nil {
		t.Fatalf("SetLogRetention: %v", err)
	}
	got, err := svc.GetLogRetention(ctx, &mcmanagerv1.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if got.KeepDays != 30 || got.MaxTotalBytes != 1<<30 {
		t.Errorf("got %+v, want KeepDays=30 MaxTotalBytes=2^30", got)
	}
}

func TestSettingsServiceSetRejectsNegative(t *testing.T) {
	svc, _ := newSettingsService(t)
	_, err := svc.SetLogRetention(context.Background(), &mcmanagerv1.LogRetention{KeepDays: -1})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", code)
	}
}

func TestSettingsServicePurgeLogsAppliesPolicy(t *testing.T) {
	svc, logsRoot := newSettingsService(t)
	ctx := context.Background()

	// One old log (8 days) and one fresh; keep_days=7 should drop only the old one.
	oldLog := filepath.Join(logsRoot, "srv1", "console-old.log")
	freshLog := filepath.Join(logsRoot, "srv1", "console-new.log")
	for _, p := range []string{oldLog, freshLog} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("12345"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldLog, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SetLogRetention(ctx, &mcmanagerv1.LogRetention{KeepDays: 7}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.PurgeLogs(ctx, &mcmanagerv1.Empty{})
	if err != nil {
		t.Fatalf("PurgeLogs: %v", err)
	}
	if res.RemovedFiles != 1 || res.FreedBytes != 5 {
		t.Errorf("PurgeResult = %+v, want RemovedFiles=1 FreedBytes=5", res)
	}
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Error("old log should have been purged")
	}
	if _, err := os.Stat(freshLog); err != nil {
		t.Error("fresh log should have been kept")
	}
}

func TestSettingsServiceGetBackendInfo(t *testing.T) {
	base := t.TempDir()
	st := settings.NewStore(filepath.Join(base, "settings.json"))
	svc := NewSettingsService(SettingsServiceConfig{
		Store:      st,
		ActiveMode: string(isolation.ModeOnMachine),
		Detector: func(context.Context) isolation.Availability {
			return isolation.Availability{Available: true, Version: "24.0.7"}
		},
	})

	info, err := svc.GetBackendInfo(context.Background(), &mcmanagerv1.Empty{})
	if err != nil {
		t.Fatalf("GetBackendInfo: %v", err)
	}
	if info.ActiveMode != string(isolation.ModeOnMachine) {
		t.Errorf("ActiveMode = %q, want on-machine", info.ActiveMode)
	}
	if !info.DockerAvailable || info.DockerVersion != "24.0.7" {
		t.Errorf("docker availability not reported: %+v", info)
	}
	if info.UseDocker {
		t.Error("UseDocker should default to false (opt-in)")
	}
}

func TestSettingsServiceSetUseDockerPersists(t *testing.T) {
	base := t.TempDir()
	st := settings.NewStore(filepath.Join(base, "settings.json"))
	svc := NewSettingsService(SettingsServiceConfig{
		Store:    st,
		Detector: func(context.Context) isolation.Availability { return isolation.Availability{} },
	})
	ctx := context.Background()

	if _, err := svc.SetUseDocker(ctx, &mcmanagerv1.UseDocker{Enabled: true}); err != nil {
		t.Fatalf("SetUseDocker: %v", err)
	}
	info, err := svc.GetBackendInfo(ctx, &mcmanagerv1.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if !info.UseDocker {
		t.Error("UseDocker preference was not persisted")
	}
	// And it is durable on disk.
	if loaded, _ := st.Load(); !loaded.UseDocker {
		t.Error("UseDocker not saved to the settings file")
	}
}
