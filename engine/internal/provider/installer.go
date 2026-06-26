package provider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// JavaResolver resolves (downloading if needed) a java.exe for a Java major
// version, reporting progress. jre.Manager.EnsureJRE satisfies this; injecting it
// keeps the provider package decoupled from the jre package.
type JavaResolver func(ctx context.Context, major int, progress func(Progress)) (string, error)

// runInstaller runs an installer jar (e.g. Forge/NeoForge --installServer),
// streaming every stdout/stderr line to progress.LogLine verbatim.
func runInstaller(ctx context.Context, javaPath, installerPath, dir string, progress func(Progress)) error {
	cmd := exec.CommandContext(ctx, javaPath, "-jar", installerPath, "--installServer")
	cmd.Dir = dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	pipe := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			report(progress, Progress{LogLine: sc.Text()})
		}
	}
	go pipe(stdout)
	go pipe(stderr)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%w: installer exited: %v", ErrInstallerFailed, err)
	}
	return nil
}

// detectServerLaunch inspects an installed Forge/NeoForge directory and returns
// the launch args. Modern installers (1.17+) generate a libraries/.../win_args.txt
// arg file launched with java @argfile; older ones leave a runnable jar.
func detectServerLaunch(dir string) ([]string, error) {
	if rel, ok := findArgsFile(dir); ok {
		return []string{"@" + rel, "nogui"}, nil
	}
	if jar, ok := findServerJar(dir); ok {
		return []string{"-jar", jar, "nogui"}, nil
	}
	return nil, fmt.Errorf("%w: no win_args.txt or server jar after install", ErrInstallerFailed)
}

func findArgsFile(dir string) (relative string, ok bool) {
	var found string
	_ = filepath.WalkDir(filepath.Join(dir, "libraries"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "win_args.txt") {
			if rel, rerr := filepath.Rel(dir, path); rerr == nil {
				found = rel
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found, found != ""
}

func findServerJar(dir string) (name string, ok bool) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.jar"))
	if err != nil {
		return "", false
	}
	for _, p := range entries {
		base := filepath.Base(p)
		lower := strings.ToLower(base)
		// Skip the installer jar itself; we want the runnable server jar.
		if strings.Contains(lower, "installer") {
			continue
		}
		if strings.HasPrefix(lower, "forge") || strings.HasPrefix(lower, "neoforge") || lower == "server.jar" {
			return base, true
		}
	}
	return "", false
}
