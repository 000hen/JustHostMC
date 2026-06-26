package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newServerDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, files)
	return dir
}

func TestManagerCreateAndList(t *testing.T) {
	m := NewManager(t.TempDir())
	src := newServerDir(t, map[string]string{"world/level.dat": "data", "server.properties": "x"})

	info, err := m.Create("srv1", src)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.ID == "" {
		t.Error("Create returned empty ID")
	}
	if info.ServerID != "srv1" {
		t.Errorf("ServerID = %q, want srv1", info.ServerID)
	}
	if info.SizeBytes <= 0 {
		t.Errorf("SizeBytes = %d, want > 0", info.SizeBytes)
	}

	list, err := m.List("srv1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != info.ID {
		t.Fatalf("List = %+v, want one entry with ID %s", list, info.ID)
	}
}

func TestManagerListNewestFirst(t *testing.T) {
	m := NewManager(t.TempDir())
	src := newServerDir(t, map[string]string{"a.txt": "a"})

	older, err := m.Create("srv1", src)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate the first archive so ordering is deterministic regardless of speed.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(m.root, "srv1", older.ID+ext), old, old); err != nil {
		t.Fatal(err)
	}

	newer, err := m.Create("srv1", src)
	if err != nil {
		t.Fatal(err)
	}

	list, err := m.List("srv1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].ID != newer.ID || list[1].ID != older.ID {
		t.Errorf("order = [%s, %s], want [%s, %s] (newest first)",
			list[0].ID, list[1].ID, newer.ID, older.ID)
	}
}

func TestManagerListUnknownServerIsEmpty(t *testing.T) {
	m := NewManager(t.TempDir())
	list, err := m.List("does-not-exist")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List = %+v, want empty", list)
	}
}

func TestManagerRestoreRoundTrip(t *testing.T) {
	m := NewManager(t.TempDir())
	src := newServerDir(t, map[string]string{"world/level.dat": "WORLD", "ops.json": "[]"})

	info, err := m.Create("srv1", src)
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "restored")
	if err := m.Restore("srv1", info.ID, dest); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := readFile(t, filepath.Join(dest, "world", "level.dat")); got != "WORLD" {
		t.Errorf("restored level.dat = %q, want WORLD", got)
	}
}

func TestManagerDelete(t *testing.T) {
	m := NewManager(t.TempDir())
	src := newServerDir(t, map[string]string{"a.txt": "a"})

	info, err := m.Create("srv1", src)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("srv1", info.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Path("srv1", info.ID); !errors.Is(err, ErrBackupNotFound) {
		t.Errorf("Path after Delete err = %v, want ErrBackupNotFound", err)
	}
	list, _ := m.List("srv1")
	if len(list) != 0 {
		t.Errorf("List after Delete = %+v, want empty", list)
	}
}

func TestManagerPathUnknownIsNotFound(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.Path("srv1", "nope"); !errors.Is(err, ErrBackupNotFound) {
		t.Errorf("Path err = %v, want ErrBackupNotFound", err)
	}
}

func TestManagerRejectsUnsafeBackupID(t *testing.T) {
	m := NewManager(t.TempDir())
	for _, bad := range []string{"../escape", "a/b", `a\b`, "..", ""} {
		if _, err := m.Path("srv1", bad); !errors.Is(err, ErrBackupNotFound) {
			t.Errorf("Path(%q) err = %v, want ErrBackupNotFound", bad, err)
		}
	}
}

func TestNewBackupIDSortsByTime(t *testing.T) {
	earlier := NewBackupID(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	later := NewBackupID(time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC))
	if !(earlier < later) {
		t.Errorf("expected %q < %q so ids sort chronologically", earlier, later)
	}
	// Two ids for the same instant must still differ (random suffix).
	now := time.Now()
	a, b := NewBackupID(now), NewBackupID(now)
	if a == b {
		t.Error("NewBackupID collided for the same timestamp")
	}
}
