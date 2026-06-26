package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectServerLaunchPrefersArgsFile(t *testing.T) {
	dir := t.TempDir()
	argsDir := filepath.Join(dir, "libraries", "net", "minecraftforge", "forge", "1.20.1-47.3.0")
	if err := os.MkdirAll(argsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(argsDir, "win_args.txt"), []byte("--args"), 0o644); err != nil {
		t.Fatal(err)
	}

	args, err := detectServerLaunch(dir)
	if err != nil {
		t.Fatalf("detectServerLaunch: %v", err)
	}
	if len(args) != 2 || !strings.HasPrefix(args[0], "@") || !strings.HasSuffix(args[0], "win_args.txt") || args[1] != "nogui" {
		t.Fatalf("args = %v, want [@...win_args.txt nogui]", args)
	}
}

func TestDetectServerLaunchJarFallbackSkipsInstaller(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"installer.jar", "forge-1.16.5-36.2.42.jar"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	args, err := detectServerLaunch(dir)
	if err != nil {
		t.Fatalf("detectServerLaunch: %v", err)
	}
	if len(args) != 3 || args[0] != "-jar" || args[1] != "forge-1.16.5-36.2.42.jar" || args[2] != "nogui" {
		t.Fatalf("args = %v, want [-jar forge-1.16.5-36.2.42.jar nogui]", args)
	}
}

func TestDetectServerLaunchNoneFound(t *testing.T) {
	if _, err := detectServerLaunch(t.TempDir()); err == nil {
		t.Fatal("expected error when nothing installed")
	}
}
