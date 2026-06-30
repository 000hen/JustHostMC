// Package appdata resolves the engine's on-disk layout under a single base
// directory, so server data, the JRE cache, logs, and backups all live in one
// place that "remove all data" can wipe (PROMPT §8).
package appdata

import (
	"os"
	"path/filepath"
)

// EnvDataDir overrides the base data directory (used by the app and tests).
const EnvDataDir = "MCMANAGER_DATA_DIR"

// Paths is the resolved directory layout.
type Paths struct {
	Base string
}

// New returns Paths rooted at base.
func New(base string) Paths { return Paths{Base: base} }

// Default resolves the base from MCMANAGER_DATA_DIR, else %LOCALAPPDATA%\JustHostMC,
// else a temp fallback.
func Default() Paths {
	if v := os.Getenv(EnvDataDir); v != "" {
		return Paths{Base: v}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return Paths{Base: filepath.Join(local, "JustHostMC")}
	}
	return Paths{Base: filepath.Join(os.TempDir(), "JustHostMC")}
}

func (p Paths) ServersRoot() string        { return filepath.Join(p.Base, "servers") }
func (p Paths) ServerDir(id string) string { return filepath.Join(p.ServersRoot(), id) }
func (p Paths) JRECache() string           { return filepath.Join(p.Base, "jre") }
func (p Paths) LogsRoot() string           { return filepath.Join(p.Base, "logs") }
func (p Paths) BackupsRoot() string        { return filepath.Join(p.Base, "backups") }
func (p Paths) ScriptsRoot() string        { return filepath.Join(p.Base, "scripts") }
