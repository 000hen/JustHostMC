// Package isolation runs and supervises per-server processes behind a backend
// abstraction. JobObjectBackend (on-machine, Windows Job Objects) is the default
// fallback; DockerBackend (M7) is used when Docker is present and the user opts in.
package isolation

import "context"

// InstanceSpec describes how to launch one server instance.
type InstanceSpec struct {
	ID        string            // stable server id
	Dir       string            // working directory (the server's data folder)
	JavaPath  string            // absolute path to java.exe (used by the on-machine backend)
	JavaMajor int               // required Java feature version (used by Docker to pick the image)
	Args      []string          // JVM + program args (e.g. -Xmx, -jar server.jar, nogui)
	MemoryMB  int               // hard memory cap enforced by the backend (0 = none)
	Port      int               // server port to publish (used by the Docker backend)
	Env       map[string]string // extra environment variables merged onto the engine's
}

// Instance is a running (or finished) server process.
type Instance interface {
	ID() string
	// WriteStdin sends one console command line to the server's stdin.
	WriteStdin(line string) error
	// Output streams merged stdout+stderr lines; closed when the process exits.
	Output() <-chan string
	// Done is closed once the process has exited.
	Done() <-chan struct{}
	// Running reports whether the process is still alive.
	Running() bool
	// ExitErr returns the process exit error (nil if it exited cleanly / is alive).
	ExitErr() error
}

// IsolationBackend starts, stops, re-adopts, and lists server instances.
type IsolationBackend interface {
	Start(ctx context.Context, spec InstanceSpec) (Instance, error)
	Stop(ctx context.Context, id string, graceful bool) error
	Attach(ctx context.Context, id string) (Instance, error) // re-adopt a known instance
	List(ctx context.Context) ([]Instance, error)
}

// Stats is a point-in-time resource snapshot for one running instance. The
// backend keeps the previous reading and reports rates directly, so callers can
// forward the values without further bookkeeping.
type Stats struct {
	CPUPercent       float64 // 0..100, normalized across all cores
	MemoryBytes      int64   // current working set
	MemoryLimitBytes int64   // configured cap; 0 = uncapped
	NetRxBytesPerSec int64
	NetTxBytesPerSec int64
	NetworkAvailable bool // false when the backend can't measure per-instance network
	TPS              float64
}

// Sampler is an optional Instance capability: callers type-assert an Instance to
// Sampler and skip metrics when it is not satisfied. Backends that can read
// resource usage (Job Object, Docker) implement it.
type Sampler interface {
	// Sample returns the latest resource snapshot. ok is false when no sample is
	// available right now (e.g. the process has exited or the backend errored).
	Sample(ctx context.Context) (stats Stats, ok bool)
}
