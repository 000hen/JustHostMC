// Package backup creates and restores portable archives of a server's data
// folder. Archives store paths relative to the server directory so they contain
// no machine-specific absolute paths (PROMPT §7).
package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// excludedNames are files that must never be archived: runtime lock files a
// running server holds open. Minecraft byte-range-locks world/session.lock, so on
// Windows it both fails to read during a live backup and is meaningless in one
// (the server regenerates it). Every server type uses the vanilla world format,
// so excluding it by name covers them all.
var excludedNames = map[string]bool{"session.lock": true}

// Archive zips srcDir into destZip (creating parent dirs), using paths relative
// to srcDir. Runtime lock files (see excludedNames) are skipped so a backup of a
// running server succeeds.
func Archive(srcDir, destZip string) error {
	if err := os.MkdirAll(filepath.Dir(destZip), 0o755); err != nil {
		return err
	}
	out, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if excludedNames[d.Name()] {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		return addFile(zw, path, filepath.ToSlash(rel))
	})
}

func addFile(zw *zip.Writer, path, name string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(w, in)
	return err
}

// Restore extracts srcZip into destDir, replacing its current contents.
func Restore(srcZip, destDir string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	cleanDest := filepath.Clean(destDir)
	for _, zf := range r.File {
		target := filepath.Join(destDir, zf.Name)
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractFile(zf, target); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(zf *zip.File, target string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
