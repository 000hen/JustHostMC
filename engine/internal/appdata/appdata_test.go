package appdata

import (
	"path/filepath"
	"testing"
)

func TestPathsLayout(t *testing.T) {
	p := New(filepath.FromSlash("C:/data/JustHostMC"))

	cases := map[string]string{
		p.ServersRoot():       filepath.FromSlash("C:/data/JustHostMC/servers"),
		p.ServerDir("abc123"): filepath.FromSlash("C:/data/JustHostMC/servers/abc123"),
		p.JRECache():          filepath.FromSlash("C:/data/JustHostMC/jre"),
		p.LogsRoot():          filepath.FromSlash("C:/data/JustHostMC/logs"),
		p.BackupsRoot():       filepath.FromSlash("C:/data/JustHostMC/backups"),
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	}
}

func TestDefaultHonorsEnv(t *testing.T) {
	t.Setenv(EnvDataDir, filepath.FromSlash("D:/custom"))
	if got := Default().Base; got != filepath.FromSlash("D:/custom") {
		t.Errorf("Default().Base = %q, want D:/custom", got)
	}
}
