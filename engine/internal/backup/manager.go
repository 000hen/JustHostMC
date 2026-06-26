package backup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ext is the on-disk extension for every backup archive.
const ext = ".zip"

// ErrBackupNotFound is returned when a backup id does not resolve to a file
// (including ids that are unsafe as filenames).
var ErrBackupNotFound = errors.New("backup not found")

// Info describes one stored backup.
type Info struct {
	ID        string
	ServerID  string
	SizeBytes int64
	CreatedAt time.Time
}

// Manager stores per-server backup archives under a single root directory,
// laid out as <root>/<serverID>/<backupID>.zip so "remove all data" can wipe
// the whole tree (PROMPT §8).
type Manager struct {
	root string
}

// NewManager returns a Manager rooted at root (typically appdata BackupsRoot).
func NewManager(root string) *Manager { return &Manager{root: root} }

func (m *Manager) serverDir(serverID string) string {
	return filepath.Join(m.root, serverID)
}

// NewBackupID returns a unique, chronologically sortable id: a UTC timestamp
// followed by a short random suffix (so ids created in the same second differ).
func NewBackupID(now time.Time) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:])
}

// safeName rejects values that are unsafe as a single path component, guarding
// against path traversal via attacker-influenced ids.
func safeName(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	return !strings.ContainsAny(s, `/\`)
}

// Create snapshots srcDir into a new backup for serverID and returns its Info.
func (m *Manager) Create(serverID, srcDir string) (Info, error) {
	if !safeName(serverID) {
		return Info{}, ErrBackupNotFound
	}
	id := NewBackupID(time.Now())
	dest := filepath.Join(m.serverDir(serverID), id+ext)
	if err := Archive(srcDir, dest); err != nil {
		return Info{}, err
	}
	fi, err := os.Stat(dest)
	if err != nil {
		return Info{}, err
	}
	return Info{ID: id, ServerID: serverID, SizeBytes: fi.Size(), CreatedAt: fi.ModTime()}, nil
}

// List returns serverID's backups, newest first. An unknown server yields an
// empty list (not an error).
func (m *Manager) List(serverID string) ([]Info, error) {
	entries, err := os.ReadDir(m.serverDir(serverID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ext {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{
			ID:        strings.TrimSuffix(e.Name(), ext),
			ServerID:  serverID,
			SizeBytes: fi.Size(),
			CreatedAt: fi.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// Path returns the archive path for a backup, or ErrBackupNotFound if it does
// not exist (or the id is unsafe).
func (m *Manager) Path(serverID, backupID string) (string, error) {
	if !safeName(serverID) || !safeName(backupID) {
		return "", ErrBackupNotFound
	}
	p := filepath.Join(m.serverDir(serverID), backupID+ext)
	if _, err := os.Stat(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrBackupNotFound
		}
		return "", err
	}
	return p, nil
}

// Restore extracts a backup into destDir, replacing its current contents.
func (m *Manager) Restore(serverID, backupID, destDir string) error {
	p, err := m.Path(serverID, backupID)
	if err != nil {
		return err
	}
	return Restore(p, destDir)
}

// Delete removes a stored backup.
func (m *Manager) Delete(serverID, backupID string) error {
	p, err := m.Path(serverID, backupID)
	if err != nil {
		return err
	}
	return os.Remove(p)
}
