package isolation

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	defaultStopTimeout = 30 * time.Second
	maxConsoleLine     = 1 << 20 // 1 MiB: truncate pathological single lines
	outputBuffer       = 4096    // bounded channel; drop under extreme backpressure
)

// JobObjectBackend runs each server in its own Windows Job Object with a hard
// memory cap and JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, so stopping the server or
// closing the engine cleanly tears down the whole process tree (no orphans).
type JobObjectBackend struct {
	mu          sync.Mutex
	instances   map[string]*jobInstance
	stopTimeout time.Duration
}

// NewJobObjectBackend creates an on-machine backend.
func NewJobObjectBackend() *JobObjectBackend {
	return &JobObjectBackend{
		instances:   make(map[string]*jobInstance),
		stopTimeout: defaultStopTimeout,
	}
}

// Start launches the instance, assigns it to a memory-capped job object, and
// begins streaming its output.
func (b *JobObjectBackend) Start(ctx context.Context, spec InstanceSpec) (Instance, error) {
	job, err := createJobObject(spec.MemoryMB)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}

	cmd := exec.Command(spec.JavaPath, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = os.Environ()
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// CREATE_NO_WINDOW: don't flash a console window for the child java process.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}

	// Assign to the job immediately after start. (A child spawned in the tiny
	// window before assignment would escape the job; java does not do this.)
	// PROCESS_QUERY_LIMITED_INFORMATION additionally lets us read CPU/memory for metrics.
	procHandle, perr := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false, uint32(cmd.Process.Pid))
	if perr == nil {
		_ = windows.AssignProcessToJobObject(job, procHandle)
	}

	inst := &jobInstance{
		id:         spec.ID,
		cmd:        cmd,
		job:        job,
		procHandle: procHandle,
		stdin:      stdin,
		output:     make(chan string, outputBuffer),
		done:       make(chan struct{}),
		memLimitMB: spec.MemoryMB,
	}

	var scanners sync.WaitGroup
	scanners.Add(2)
	go inst.scan(&scanners, stdout)
	go inst.scan(&scanners, stderr)
	go inst.reap(&scanners)

	b.mu.Lock()
	b.instances[spec.ID] = inst
	b.mu.Unlock()
	return inst, nil
}

// Stop stops an instance. When graceful, it sends the "stop" console command and
// waits up to stopTimeout before force-terminating the job.
func (b *JobObjectBackend) Stop(ctx context.Context, id string, graceful bool) error {
	b.mu.Lock()
	inst, ok := b.instances[id]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("instance %q not found", id)
	}
	if !inst.Running() {
		return nil
	}

	if graceful {
		_ = inst.WriteStdin("stop")
		select {
		case <-inst.done:
			return nil
		case <-time.After(b.stopTimeout):
			// timed out; fall through to force kill
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return inst.terminate()
}

func (b *JobObjectBackend) Attach(_ context.Context, id string) (Instance, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if inst, ok := b.instances[id]; ok {
		return inst, nil
	}
	// Cross-restart re-adoption of on-machine instances is handled in M3.
	return nil, fmt.Errorf("instance %q not found", id)
}

func (b *JobObjectBackend) List(_ context.Context) ([]Instance, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Instance, 0, len(b.instances))
	for _, inst := range b.instances {
		out = append(out, inst)
	}
	return out, nil
}

// jobInstance is one running server.
type jobInstance struct {
	id         string
	cmd        *exec.Cmd
	job        windows.Handle
	procHandle windows.Handle
	stdin      io.WriteCloser
	output     chan string
	done       chan struct{}
	memLimitMB int
	tps        tpsTracker

	mu      sync.Mutex
	exitErr error

	// metMu guards the previous CPU reading used to compute CPU percent across
	// successive Sample calls.
	metMu     sync.Mutex
	lastCPU   time.Duration
	lastCPUAt time.Time
}

func (i *jobInstance) ID() string            { return i.id }
func (i *jobInstance) Output() <-chan string { return i.output }
func (i *jobInstance) Done() <-chan struct{} { return i.done }

func (i *jobInstance) Running() bool {
	select {
	case <-i.done:
		return false
	default:
		return true
	}
}

func (i *jobInstance) ExitErr() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.exitErr
}

func (i *jobInstance) WriteStdin(line string) error {
	_, err := io.WriteString(i.stdin, line+"\n")
	return err
}

func (i *jobInstance) scan(wg *sync.WaitGroup, r io.Reader) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxConsoleLine)
	for sc.Scan() {
		i.tps.Observe(sc.Text())
		select {
		case i.output <- sc.Text():
		default:
			// Consumer is overwhelmed; drop rather than stall the server's pipe.
		}
	}
}

// reap waits for the process to exit, then closes the output, releases handles
// (closing the job kills any survivors via KILL_ON_JOB_CLOSE), and signals done.
func (i *jobInstance) reap(scanners *sync.WaitGroup) {
	err := i.cmd.Wait()
	scanners.Wait()

	i.mu.Lock()
	i.exitErr = err
	i.mu.Unlock()

	close(i.output)
	windows.CloseHandle(i.job)
	if i.procHandle != 0 {
		windows.CloseHandle(i.procHandle)
	}
	close(i.done)
}

func (i *jobInstance) terminate() error {
	err := windows.TerminateJobObject(i.job, 1)
	select {
	case <-i.done:
	case <-time.After(5 * time.Second):
	}
	return err
}

func createJobObject(memoryMB int) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if memoryMB > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY
		info.JobMemoryLimit = uintptr(memoryMB) * 1024 * 1024
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// ── Metrics sampling ──────────────────────────────────────────────────────────

var (
	modPSAPI                 = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo = modPSAPI.NewProc("GetProcessMemoryInfo")
)

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS (psapi.h). Only
// WorkingSetSize is read; the rest are present so the struct size matches.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// Sample reports CPU and memory for the server's main java process. CPU percent
// is derived from the change in process CPU time since the previous call, so the
// first sample after Start reads 0%. Per-process network usage isn't practical on
// Windows, so NetworkAvailable is always false here — use the Docker backend for
// network metrics.
func (i *jobInstance) Sample(_ context.Context) (Stats, bool) {
	if !i.Running() || i.procHandle == 0 {
		return Stats{}, false
	}

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(i.procHandle, &creation, &exit, &kernel, &user); err != nil {
		return Stats{}, false
	}
	cpu := time.Duration(kernel.Nanoseconds() + user.Nanoseconds())
	now := time.Now()

	i.metMu.Lock()
	var pct float64
	if !i.lastCPUAt.IsZero() {
		if wall := now.Sub(i.lastCPUAt); wall > 0 {
			pct = float64(cpu-i.lastCPU) / float64(wall) / float64(runtime.NumCPU()) * 100
		}
	}
	i.lastCPU, i.lastCPUAt = cpu, now
	i.metMu.Unlock()

	stats := Stats{
		CPUPercent:       pct,
		MemoryLimitBytes: int64(i.memLimitMB) * 1024 * 1024,
	}
	if mem, ok := processWorkingSet(i.procHandle); ok {
		stats.MemoryBytes = mem
	}
	stats.TPS = i.tps.Value()
	return stats, true
}

// processWorkingSet returns the process working-set size in bytes via psapi.
func processWorkingSet(h windows.Handle) (int64, bool) {
	var pmc processMemoryCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
	if r == 0 {
		return 0, false
	}
	return int64(pmc.workingSetSize), true
}
