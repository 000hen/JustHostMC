package backup

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeTree creates files (relative path -> contents) under root.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestArchiveRestoreRoundTrip(t *testing.T) {
	src := t.TempDir()
	files := map[string]string{
		"server.properties":  "server-port=25565\n",
		"world/level.dat":    "\x00\x01binary\x02",
		"world/region/r.0.0": "regiondata",
		"logs/latest.log":    "[INFO] started",
	}
	writeTree(t, src, files)

	zipPath := filepath.Join(t.TempDir(), "snap.zip")
	if err := Archive(src, zipPath); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if fi, err := os.Stat(zipPath); err != nil || fi.Size() == 0 {
		t.Fatalf("archive missing or empty: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "restored")
	if err := Restore(zipPath, dest); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	for rel, want := range files {
		got := readFile(t, filepath.Join(dest, filepath.FromSlash(rel)))
		if got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestArchiveExcludesSessionLock(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"world/level.dat":    "data",
		"world/session.lock": "lock", // held open + byte-range locked by a live server
	})

	zipPath := filepath.Join(t.TempDir(), "snap.zip")
	if err := Archive(src, zipPath); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "restored")
	if err := Restore(zipPath, dest); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "world", "level.dat")); err != nil {
		t.Errorf("world/level.dat should be archived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "world", "session.lock")); !os.IsNotExist(err) {
		t.Errorf("session.lock should be excluded from the archive (err=%v)", err)
	}
}

func TestRestoreReplacesExistingContents(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"keep.txt": "new"})

	zipPath := filepath.Join(t.TempDir(), "snap.zip")
	if err := Archive(src, zipPath); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	// A stale file that is not in the archive must be gone after restore.
	writeTree(t, dest, map[string]string{"stale.txt": "old"})

	if err := Restore(zipPath, dest); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "stale.txt")); !os.IsNotExist(err) {
		t.Errorf("stale file survived restore (err=%v)", err)
	}
	if got := readFile(t, filepath.Join(dest, "keep.txt")); got != "new" {
		t.Errorf("keep.txt = %q, want %q", got, "new")
	}
}

func TestRestoreRejectsZipSlip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	w, err := zw.Create("../escape.txt") // path traversal attempt
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("pwned")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()

	dest := filepath.Join(t.TempDir(), "restored")
	if err := Restore(zipPath, dest); err == nil {
		t.Fatal("expected Restore to reject a path-traversal entry, got nil")
	}
	// The escape target must not have been written outside dest.
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.txt")); !os.IsNotExist(err) {
		t.Errorf("zip-slip wrote outside dest (err=%v)", err)
	}
}
