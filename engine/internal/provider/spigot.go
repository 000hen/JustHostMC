package provider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/000hen/justhostmc/engine/internal/dl"
)

const (
	// defaultBuildToolsURL points to the latest successful BuildTools artifact
	// from SpigotMC's Jenkins CI.
	defaultBuildToolsURL = "https://hub.spigotmc.org/jenkins/job/BuildTools/lastSuccessfulBuild/artifact/target/BuildTools.jar"
	// defaultMojangManifest is the same version manifest used by the Vanilla
	// provider; Spigot (via BuildTools) supports all release versions.
	defaultMojangManifest = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
)

// Spigot builds a Spigot server jar from source using SpigotMC's BuildTools.
// BuildTools clones the Bukkit/CraftBukkit/Spigot repositories, applies patches,
// and compiles the server jar. This requires Git to be installed on the system.
type Spigot struct {
	client       *http.Client
	buildToolsURL string
	manifestURL  string
	java         JavaResolver
}

type SpigotOption func(*Spigot)

func WithSpigotHTTPClient(c *http.Client) SpigotOption {
	return func(s *Spigot) { s.client = c }
}
func WithSpigotBuildToolsURL(u string) SpigotOption {
	return func(s *Spigot) { s.buildToolsURL = u }
}
func WithSpigotManifestURL(u string) SpigotOption {
	return func(s *Spigot) { s.manifestURL = u }
}

// NewSpigot builds a Spigot provider. java resolves the JRE needed to run
// BuildTools (and later the server).
func NewSpigot(java JavaResolver, opts ...SpigotOption) *Spigot {
	s := &Spigot{
		client:        http.DefaultClient,
		buildToolsURL: defaultBuildToolsURL,
		manifestURL:   defaultMojangManifest,
		java:          java,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Versions returns all release Minecraft versions (newest first) since
// BuildTools supports any release version.
func (s *Spigot) Versions(ctx context.Context) ([]string, error) {
	var m struct {
		Versions []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"versions"`
	}
	if err := getJSON(ctx, s.client, s.manifestURL, &m); err != nil {
		return nil, err
	}
	var out []string
	for _, v := range m.Versions {
		if v.Type == "release" {
			out = append(out, v.ID)
		}
	}
	// Mojang's manifest is already sorted newest-first, so no extra sorting.
	return out, nil
}

// Install downloads BuildTools.jar and runs it to compile a Spigot server jar
// for the requested Minecraft version. Git must be installed on the system.
func (s *Spigot) Install(ctx context.Context, dir, version string, progress func(Progress)) (LaunchSpec, error) {
	// -- 0. Pre-flight: Git is required -------------------------------------------
	report(progress, Progress{Step: "install.progress.preparing", Fraction: -1})
	if _, err := exec.LookPath("git"); err != nil {
		return LaunchSpec{}, fmt.Errorf(
			"%w: BuildTools requires Git. Please install Git from https://git-scm.com/downloads and ensure it is in your PATH",
			ErrInstallerFailed,
		)
	}
	report(progress, Progress{LogLine: "Git found in PATH"})

	// -- 1. Resolve Java ----------------------------------------------------------
	major := JavaMajorForMC(version)
	report(progress, Progress{Step: "install.progress.preparing", Fraction: -1})
	javaPath, err := s.java(ctx, major, progress)
	if err != nil {
		return LaunchSpec{}, err
	}

	// -- 2. Download BuildTools.jar -----------------------------------------------
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return LaunchSpec{}, err
	}
	buildToolsPath := filepath.Join(dir, "BuildTools.jar")
	report(progress, Progress{Step: "install.progress.downloading_installer", Fraction: 0, LogLine: "BuildTools.jar"})
	_, _, err = dl.Download(ctx, s.client, s.buildToolsURL, buildToolsPath, nil, func(done, total int64) {
		frac := -1.0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
		report(progress, Progress{Fraction: frac})
	})
	if err != nil {
		return LaunchSpec{}, fmt.Errorf("download BuildTools.jar: %w", err)
	}

	// -- 3. Run BuildTools --------------------------------------------------------
	report(progress, Progress{Step: "install.progress.running_installer", Fraction: -1, LogLine: fmt.Sprintf("java -jar BuildTools.jar --rev %s", version)})
	if err := runBuildTools(ctx, javaPath, buildToolsPath, dir, version, progress); err != nil {
		return LaunchSpec{}, err
	}

	// -- 4. Locate the output jar -------------------------------------------------
	jarName, err := findSpigotJar(dir)
	if err != nil {
		return LaunchSpec{}, err
	}
	report(progress, Progress{LogLine: fmt.Sprintf("Built: %s", jarName)})

	report(progress, Progress{Step: "install.progress.done", Fraction: 1})
	return LaunchSpec{JavaMajor: major, Args: []string{"-jar", jarName, "nogui"}}, nil
}

// runBuildTools executes BuildTools.jar with --rev, streaming stdout/stderr as
// log lines. BuildTools compiles from source so this typically takes 5–15 min
// and produces extensive output.
func runBuildTools(ctx context.Context, javaPath, buildToolsPath, dir, version string, progress func(Progress)) error {
	cmd := exec.CommandContext(ctx, javaPath, "-jar", buildToolsPath, "--rev", version)
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
		return fmt.Errorf("%w: BuildTools failed to start: %v", ErrInstallerFailed, err)
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
		return fmt.Errorf("%w: BuildTools exited: %v", ErrInstallerFailed, err)
	}
	return nil
}

// findSpigotJar locates the spigot-*.jar produced by BuildTools in dir.
func findSpigotJar(dir string) (string, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "spigot-*.jar"))
	if err != nil {
		return "", err
	}
	for _, p := range entries {
		base := filepath.Base(p)
		lower := strings.ToLower(base)
		// Skip shaded/sources jars that BuildTools may produce alongside.
		if strings.Contains(lower, "shaded") || strings.Contains(lower, "sources") {
			continue
		}
		return base, nil
	}
	// Fallback: some older BuildTools versions may name it craftbukkit-*.jar
	entries, _ = filepath.Glob(filepath.Join(dir, "craftbukkit-*.jar"))
	for _, p := range entries {
		base := filepath.Base(p)
		lower := strings.ToLower(base)
		if strings.Contains(lower, "shaded") || strings.Contains(lower, "sources") {
			continue
		}
		return base, nil
	}
	return "", fmt.Errorf("%w: no spigot-*.jar found after BuildTools completed", ErrInstallerFailed)
}
