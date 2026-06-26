package isolation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// dockerLabel tags every container the engine creates so it can find and re-adopt
// only its own containers (never touching the user's other containers).
const dockerLabel = "com.justhostmc.managed"

// containerPrefix namespaces our container names by server id.
const containerPrefix = "jhmc-"

// BackendMode names which isolation backend is active, for transparent reporting
// to the user (PROMPT §10.7).
type BackendMode string

const (
	ModeOnMachine BackendMode = "on-machine" // Windows Job Objects (default fallback)
	ModeDocker    BackendMode = "docker"     // Docker Desktop containers (opt-in)
)

// Availability reports whether the Docker engine is usable. Reason is diagnostic
// (logged), never shown to users as prose.
type Availability struct {
	Available bool
	Version   string
	Reason    string
}

// Runner runs a command to completion and returns its combined output. It is
// abstracted so detection can be unit-tested without a real Docker install.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// execRunner is the production Runner backed by os/exec.
func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// DetectDocker reports whether a usable Docker engine is present. It never starts
// or installs anything — it only queries the daemon (PROMPT §8: never auto-install
// Docker/WSL/Hyper-V).
func DetectDocker(ctx context.Context, run Runner) Availability {
	if run == nil {
		run = execRunner
	}
	out, err := run(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return Availability{Reason: "docker engine not reachable: " + firstLine(string(out)+err.Error())}
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return Availability{Reason: "docker reported no server version (daemon not running?)"}
	}
	return Availability{Available: true, Version: v}
}

// SelectMode chooses the backend: Docker only when the user has opted in AND the
// engine is available; otherwise the on-machine fallback. Docker is never used
// without explicit consent (PROMPT §10.7).
func SelectMode(consentDocker bool, avail Availability) BackendMode {
	if consentDocker && avail.Available {
		return ModeDocker
	}
	return ModeOnMachine
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// containerName returns the container name for a server id.
func containerName(id string) string { return containerPrefix + id }

// idFromContainerName recovers the server id from a managed container name, or ""
// if the name is not one of ours.
func idFromContainerName(name string) string {
	name = strings.TrimPrefix(name, "/") // docker ps may prefix with '/'
	if !strings.HasPrefix(name, containerPrefix) {
		return ""
	}
	return strings.TrimPrefix(name, containerPrefix)
}

// dockerImage returns the JRE image for a Java major version.
func dockerImage(javaMajor int) string {
	if javaMajor <= 0 {
		javaMajor = 21
	}
	return fmt.Sprintf("eclipse-temurin:%d-jre", javaMajor)
}

// dockerRunArgs builds the `docker run` arguments for a server instance. The
// container runs detached (-d) and interactive (-i) so it survives an engine
// restart (enabling re-adoption) while still accepting console commands on stdin.
func dockerRunArgs(spec InstanceSpec) []string {
	args := []string{
		"run", "-d", "-i",
		"--name", containerName(spec.ID),
		"--label", dockerLabel + "=1",
		"--label", dockerLabel + ".id=" + spec.ID,
		"-v", spec.Dir + ":/data",
		"-w", "/data",
	}
	if spec.MemoryMB > 0 {
		args = append(args, "-m", fmt.Sprintf("%dm", spec.MemoryMB))
	}
	if spec.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("%d:%d", spec.Port, spec.Port))
	}
	args = append(args, dockerImage(spec.JavaMajor), "java")
	args = append(args, spec.Args...)
	return args
}

// dockerStopArgs builds the args to stop a container (graceful = SIGTERM with a
// timeout, which the Minecraft server traps to save and shut down cleanly; force =
// immediate kill).
func dockerStopArgs(id string, graceful bool) []string {
	if graceful {
		return []string{"stop", containerName(id)}
	}
	return []string{"kill", containerName(id)}
}

// dockerPSArgs lists running managed containers, one name per line.
func dockerPSArgs() []string {
	return []string{"ps", "--filter", "label=" + dockerLabel, "--format", "{{.Names}}"}
}

// parsePSNames turns `docker ps` name output into server ids.
func parsePSNames(out string) []string {
	var ids []string
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if id := idFromContainerName(strings.TrimSpace(line)); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// DockerBackend runs each server as a Docker container (opt-in, M7). It implements
// the same IsolationBackend contract as the on-machine Job Object backend, so the
// rest of the engine is backend-agnostic. The live container lifecycle requires a
// running Docker Desktop; selection only routes here after DetectDocker succeeds
// and the user has consented.
type DockerBackend struct {
	run Runner
}

// NewDockerBackend builds a DockerBackend using the real docker CLI.
func NewDockerBackend() *DockerBackend { return &DockerBackend{run: execRunner} }

// Start creates and starts a container for the server, then streams its logs.
func (b *DockerBackend) Start(ctx context.Context, spec InstanceSpec) (Instance, error) {
	// Remove any stale container with the same name (e.g. left by a crash).
	_, _ = b.run(ctx, "docker", "rm", "-f", containerName(spec.ID))

	if out, err := b.run(ctx, "docker", dockerRunArgs(spec)...); err != nil {
		return nil, fmt.Errorf("docker run: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return newDockerInstance(spec.ID), nil
}

// Stop stops a running container (graceful SIGTERM or forced kill).
func (b *DockerBackend) Stop(ctx context.Context, id string, graceful bool) error {
	if out, err := b.run(ctx, "docker", dockerStopArgs(id, graceful)...); err != nil {
		return fmt.Errorf("docker stop: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Attach re-adopts a container that is still running after an engine restart.
func (b *DockerBackend) Attach(_ context.Context, id string) (Instance, error) {
	return newDockerInstance(id), nil
}

// List returns the managed containers Docker currently reports as running.
func (b *DockerBackend) List(ctx context.Context) ([]Instance, error) {
	out, err := b.run(ctx, "docker", dockerPSArgs()...)
	if err != nil {
		return nil, fmt.Errorf("docker ps: %v", err)
	}
	var insts []Instance
	for _, id := range parsePSNames(string(out)) {
		insts = append(insts, newDockerInstance(id))
	}
	return insts, nil
}

// dockerInstance is a handle to one running container. Its Output is fed by a
// `docker logs -f` stream and its stdin by `docker attach`.
type dockerInstance struct {
	id   string
	out  chan string
	done chan struct{}

	mu     sync.Mutex
	stdin  *bufio.Writer
	closed bool

	// metMu guards the previous cumulative network counters used to derive the
	// per-second network rates across successive Sample calls.
	metMu     sync.Mutex
	lastRx    int64
	lastTx    int64
	lastNetAt time.Time
}

func newDockerInstance(id string) *dockerInstance {
	d := &dockerInstance{id: id, out: make(chan string, 256), done: make(chan struct{})}
	go d.stream()
	return d
}

// stream pipes `docker logs -f` into the output channel until the container exits.
func (d *dockerInstance) stream() {
	defer close(d.out)
	cmd := exec.Command("docker", "logs", "-f", "--since", "0", containerName(d.id))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		select {
		case d.out <- sc.Text():
		default: // drop if nobody is reading, never block the stream
		}
	}
	_ = cmd.Wait()
	d.markDone()
}

func (d *dockerInstance) markDone() {
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		close(d.done)
	}
	d.mu.Unlock()
}

func (d *dockerInstance) ID() string            { return d.id }
func (d *dockerInstance) Output() <-chan string { return d.out }
func (d *dockerInstance) Done() <-chan struct{} { return d.done }
func (d *dockerInstance) ExitErr() error        { return nil }

func (d *dockerInstance) Running() bool {
	select {
	case <-d.done:
		return false
	default:
		return true
	}
}

// WriteStdin sends a console command to the container's main process via
// `docker attach` (the container was started with -i).
func (d *dockerInstance) WriteStdin(line string) error {
	cmd := exec.Command("docker", "exec", "-i", containerName(d.id), "sh", "-c",
		"cat > /proc/1/fd/0")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_, werr := stdin.Write([]byte(line + "\n"))
	_ = stdin.Close()
	_ = cmd.Wait()
	return werr
}

// ── Metrics sampling ──────────────────────────────────────────────────────────

// Sample reads `docker stats` for the container. Docker reports CPU percent and
// current memory directly; network counters are cumulative, so the rate is
// derived from the change since the previous call (the first sample reads 0/s).
func (d *dockerInstance) Sample(ctx context.Context) (Stats, bool) {
	if !d.Running() {
		return Stats{}, false
	}
	out, err := exec.CommandContext(ctx, "docker", "stats", "--no-stream",
		"--format", "{{json .}}", containerName(d.id)).Output()
	if err != nil {
		return Stats{}, false
	}

	var raw struct {
		CPUPerc  string `json:"CPUPerc"`
		MemUsage string `json:"MemUsage"`
		NetIO    string `json:"NetIO"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &raw); err != nil {
		return Stats{}, false
	}

	memUsed, memLimit := parseUsagePair(raw.MemUsage)
	rx, tx := parseUsagePair(raw.NetIO)
	now := time.Now()

	d.metMu.Lock()
	var rxRate, txRate int64
	if !d.lastNetAt.IsZero() {
		if dt := now.Sub(d.lastNetAt).Seconds(); dt > 0 {
			rxRate = perSecond(rx-d.lastRx, dt)
			txRate = perSecond(tx-d.lastTx, dt)
		}
	}
	d.lastRx, d.lastTx, d.lastNetAt = rx, tx, now
	d.metMu.Unlock()

	return Stats{
		CPUPercent:       parsePercent(raw.CPUPerc),
		MemoryBytes:      memUsed,
		MemoryLimitBytes: memLimit,
		NetRxBytesPerSec: rxRate,
		NetTxBytesPerSec: txRate,
		NetworkAvailable: true,
	}, true
}

func perSecond(delta int64, seconds float64) int64 {
	if delta <= 0 || seconds <= 0 {
		return 0
	}
	return int64(float64(delta) / seconds)
}

// parsePercent parses docker's "12.34%" CPU field.
func parsePercent(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	return v
}

// parseUsagePair parses docker's "used / limit" fields (memory, net I/O) into two
// byte counts, e.g. "512MiB / 2GiB" or "1.2kB / 3.4MB".
func parseUsagePair(s string) (left, right int64) {
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		return 0, 0
	}
	return parseSize(a), parseSize(b)
}

// parseSize parses a docker size string like "512MiB", "2GiB", "1.2kB", or "0B"
// into bytes, accepting both binary (KiB/MiB/GiB) and decimal (kB/MB/GB) units.
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && (s[end] == '.' || s[end] == '-' || (s[end] >= '0' && s[end] <= '9')) {
		end++
	}
	num, err := strconv.ParseFloat(strings.TrimSpace(s[:end]), 64)
	if err != nil {
		return 0
	}
	return int64(num * unitMultiplier(strings.ToLower(strings.TrimSpace(s[end:]))))
}

func unitMultiplier(unit string) float64 {
	switch unit {
	case "kb":
		return 1e3
	case "kib":
		return 1 << 10
	case "mb":
		return 1e6
	case "mib":
		return 1 << 20
	case "gb":
		return 1e9
	case "gib":
		return 1 << 30
	case "tb":
		return 1e12
	case "tib":
		return 1 << 40
	default: // "", "b", and anything unexpected
		return 1
	}
}
