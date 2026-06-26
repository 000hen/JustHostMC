package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeLog(t *testing.T, path string, size int, modAge time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := time.Now().Add(-modAge)
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestLoggerAppendsAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "srv1", "console.log")

	lg, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = lg.WriteLine("line one")
	_ = lg.WriteLine("line two")
	lg.Close()

	// Re-open must append, not truncate.
	lg2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = lg2.WriteLine("line three")
	lg2.Close()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := "line one\nline two\nline three\n"
	if got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
}

func TestPurgeDeletesByAge(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "srv1", "console-old.log")
	fresh := filepath.Join(root, "srv1", "console-new.log")
	writeLog(t, old, 100, 10*24*time.Hour)  // 10 days old
	writeLog(t, fresh, 100, 1*time.Hour)    // 1 hour old

	removed, freed, err := Purge(root, Policy{KeepDays: 7}, time.Now())
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if removed != 1 || freed != 100 {
		t.Errorf("removed=%d freed=%d, want 1 and 100", removed, freed)
	}
	if exists(old) {
		t.Error("old log should have been purged")
	}
	if !exists(fresh) {
		t.Error("fresh log should have been kept")
	}
}

func TestPurgeEnforcesSizeCapOldestFirst(t *testing.T) {
	root := t.TempDir()
	// Three 100-byte logs at distinct ages; cap of 250 bytes must drop the oldest.
	a := filepath.Join(root, "s", "a.log") // oldest
	b := filepath.Join(root, "s", "b.log")
	c := filepath.Join(root, "s", "c.log") // newest
	writeLog(t, a, 100, 3*time.Hour)
	writeLog(t, b, 100, 2*time.Hour)
	writeLog(t, c, 100, 1*time.Hour)

	removed, freed, err := Purge(root, Policy{MaxTotalBytes: 250}, time.Now())
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if removed != 1 || freed != 100 {
		t.Errorf("removed=%d freed=%d, want 1 and 100", removed, freed)
	}
	if exists(a) {
		t.Error("oldest log should have been purged to meet the size cap")
	}
	if !exists(b) || !exists(c) {
		t.Error("newer logs should have been kept")
	}
}

func TestPurgeAgeThenSize(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "s", "old.log")
	b := filepath.Join(root, "s", "b.log")
	c := filepath.Join(root, "s", "c.log")
	writeLog(t, old, 100, 30*24*time.Hour) // purged by age
	writeLog(t, b, 100, 2*time.Hour)
	writeLog(t, c, 100, 1*time.Hour)

	// After age removes "old" (200 left), cap 150 must also drop "b".
	removed, _, err := Purge(root, Policy{KeepDays: 7, MaxTotalBytes: 150}, time.Now())
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed=%d, want 2 (one by age, one by size)", removed)
	}
	if exists(old) || exists(b) {
		t.Error("old (age) and b (size) should both be gone")
	}
	if !exists(c) {
		t.Error("newest log should remain")
	}
}

func TestPurgeNoPolicyKeepsEverything(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "s", "keep.log")
	writeLog(t, p, 100, 100*24*time.Hour)

	removed, freed, err := Purge(root, Policy{}, time.Now())
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if removed != 0 || freed != 0 {
		t.Errorf("removed=%d freed=%d, want 0 and 0 with an empty policy", removed, freed)
	}
	if !exists(p) {
		t.Error("log must be kept when no limits are set")
	}
}

func TestPurgeMissingRootIsNoError(t *testing.T) {
	removed, _, err := Purge(filepath.Join(t.TempDir(), "nope"), Policy{KeepDays: 1}, time.Now())
	if err != nil {
		t.Fatalf("Purge on missing root: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed=%d, want 0", removed)
	}
}
