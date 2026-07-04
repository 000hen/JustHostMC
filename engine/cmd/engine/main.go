// Command engine is the JustHostMC backend daemon. The WinUI app launches it as
// a child process and communicates over a Windows Named Pipe. It is not meant to
// be run directly by users.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/backup"
	"github.com/000hen/justhostmc/engine/internal/console"
	grpcsvc "github.com/000hen/justhostmc/engine/internal/grpc"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/logging"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/settings"
	"github.com/000hen/justhostmc/engine/internal/store"
)

const (
	// pipeEnvVar carries the named-pipe name supplied by the app.
	pipeEnvVar = "MCMANAGER_PIPE"
	// readyLine is printed to stdout once the engine is listening.
	readyLine = "MCMANAGER_READY"
)

func main() {
	// Logs go to stderr so they never collide with the ready handshake on stdout.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	pipeName := os.Getenv(pipeEnvVar)
	if pipeName == "" {
		log.Fatalf("%s is required: the app supplies a named-pipe name", pipeEnvVar)
	}

	lis, err := grpcsvc.ListenPipe(pipeName)
	if err != nil {
		log.Fatalf("listen pipe: %v", err)
	}

	paths := appdata.Default()
	if err := os.MkdirAll(paths.Base, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	registry, err := store.OpenSQLite(filepath.Join(paths.Base, "registry.db"))
	if err != nil {
		log.Fatalf("open registry: %v", err)
	}
	defer registry.Close()

	jreMgr := jre.NewManager(paths.JRECache())
	// Server types are now sandboxed Lua provider scripts. Built-ins are embedded
	// in the binary; user-imported scripts are loaded from the data dir below.
	host := scripting.NewHost(nil, jreMgr.EnsureJRE, jreMgr.EnsureJDK)
	grants := scripting.NewGrantStore(filepath.Join(paths.Base, "grants.json"))
	providers := scripting.NewRegistry(host, grants)
	if err := scripting.LoadBuiltins(providers); err != nil {
		log.Fatalf("load builtin providers: %v", err)
	}
	providersDir := filepath.Join(paths.Base, "providers")
	if err := scripting.LoadUserProviders(providers, providersDir); err != nil {
		log.Printf("load user providers: %v", err)
	}
	// Persist every console line to a daily-rotating per-server log file.
	hub := console.NewHub()
	sink := logging.NewSink(paths.LogsRoot())
	hub.SetLineObserver(sink.Write)

	// Choose the isolation backend: Docker only when the user opted in AND it is
	// available, otherwise the on-machine Job Object backend (PROMPT 禮10.7).
	settingsStore := settings.NewStore(filepath.Join(paths.Base, "settings.json"))
	// Purge before restoring automation history so expired sessions are never
	// replayed into the UI during this run.
	applyLogRetention(settingsStore, paths.LogsRoot(), sink.CloseAll)
	automationLogs, err := scripting.NewPersistentLogBuffer(
		0, filepath.Join(paths.LogsRoot(), "automation"))
	if err != nil {
		log.Fatalf("open automation logs: %v", err)
	}
	closeLogs := func() {
		sink.CloseAll()
		automationLogs.Close()
	}
	defer closeLogs()

	backend, activeMode := selectBackend(context.Background(), settingsStore)
	log.Printf("isolation backend: %s", activeMode)

	serverService := grpcsvc.NewServerService(grpcsvc.ServerServiceConfig{
		Store:     registry,
		Providers: providers,
		JRE:       jreMgr,
		Backend:   backend,
		Paths:     paths,
		Console:   hub,
		CloseLogs: closeLogs,
	})
	// Reclaim state from the persisted registry after a restart.
	serverService.Reconcile(context.Background())

	backupService := grpcsvc.NewBackupService(grpcsvc.BackupServiceConfig{
		Manager: backup.NewManager(paths.BackupsRoot()),
		Store:   registry,
		Paths:   paths,
		Console: hub,
	})

	settingsService := grpcsvc.NewSettingsService(grpcsvc.SettingsServiceConfig{
		Store:      settingsStore,
		LogsRoot:   paths.LogsRoot(),
		ActiveMode: string(activeMode),
		CloseLogs:  closeLogs,
	})
	// The startup pass ran before history was restored; continue periodically.
	go runLogJanitor(settingsStore, paths.LogsRoot(), closeLogs)

	// Automation scripts drive running servers via the console hub and the
	// server service. They are sandboxed and permission-gated like providers.
	scriptGrants := scripting.NewGrantStore(filepath.Join(paths.Base, "script-grants.json"))
	scriptsEnabled := scripting.NewEnabledStore(filepath.Join(paths.Base, "scripts-enabled.json"))
	automation := scripting.NewManager(host, scriptGrants, hub, serverService, automationLogs)
	scriptsDir := paths.ScriptsRoot()
	if err := scripting.LoadUserScripts(automation, scriptsDir); err != nil {
		log.Printf("load automation scripts: %v", err)
	}
	for _, id := range scriptsEnabled.EnabledIDs() {
		if err := automation.Enable(id); err != nil {
			log.Printf("enable automation %q: %v", id, err)
		}
	}
	defer automation.Shutdown()

	srv := grpcsvc.NewServer(grpcsvc.Config{
		Providers:       providers,
		ServerService:   serverService,
		ConsoleService:  grpcsvc.NewConsoleService(hub),
		BackupService:   backupService,
		SettingsService: settingsService,
		PlayerService:   grpcsvc.NewPlayerService(hub, registry, paths),
		MetricsService:  grpcsvc.NewMetricsService(serverService),
		ModService:      grpcsvc.NewModService(registry, paths),
		ConfigService:   grpcsvc.NewConfigService(registry, paths),
		ProviderService: grpcsvc.NewProviderService(providers, grants, providersDir),
		ScriptService:   grpcsvc.NewScriptService(automation, scriptGrants, scriptsEnabled, scriptsDir),
	})
	log.Printf("engine data dir: %s", paths.Base)

	// Signal readiness to the parent process.
	fmt.Println(readyLine)
	_ = os.Stdout.Sync()
	log.Printf("engine listening on pipe: %s", pipeName)

	go waitForShutdown(srv)

	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// selectBackend chooses the isolation backend from the user's persisted Docker
// opt-in and live Docker availability, falling back to the on-machine Job Object
// backend. Docker is never used without consent and is never auto-installed
// (PROMPT 禮8, 禮10.7).
func selectBackend(ctx context.Context, settingsStore *settings.Store) (isolation.IsolationBackend, isolation.BackendMode) {
	consent := false
	if s, err := settingsStore.Load(); err == nil {
		consent = s.UseDocker
	}
	avail := isolation.DetectDocker(ctx, nil)
	mode := isolation.SelectMode(consent, avail)
	if mode == isolation.ModeDocker {
		return isolation.NewDockerBackend(), mode
	}
	if consent && !avail.Available {
		log.Printf("docker requested but unavailable (%s); using on-machine backend", avail.Reason)
	}
	return isolation.NewJobObjectBackend(), mode
}

// logRetentionInterval is how often the engine re-applies the log retention
// policy in the background (in addition to the immediate purge at startup).
const logRetentionInterval = 6 * time.Hour

// runLogJanitor reapplies the stored retention policy on a timer for the life
// of the process (PROMPT 禮15). The startup pass is synchronous in main so
// expired automation sessions are gone before persistent history is loaded.
func runLogJanitor(settingsStore *settings.Store, logsRoot string, closeLogs func()) {
	ticker := time.NewTicker(logRetentionInterval)
	defer ticker.Stop()
	for range ticker.C {
		applyLogRetention(settingsStore, logsRoot, closeLogs)
	}
}

func applyLogRetention(settingsStore *settings.Store, logsRoot string, closeLogs func()) {
	s, err := settingsStore.Load()
	if err != nil {
		log.Printf("log retention: load settings: %v", err)
		return
	}
	if closeLogs != nil {
		closeLogs()
	}
	removed, freed, err := logging.Purge(logsRoot, s.Policy(), time.Now())
	if err != nil {
		log.Printf("log retention: purge: %v", err)
	}
	if removed > 0 {
		log.Printf("log retention: purged %d files (%d bytes freed)", removed, freed)
	}
}

// waitForShutdown gracefully stops the server when the OS signals termination or
// when the parent process goes away (our stdin reaches EOF). The latter guards
// against leaking the engine if the app crashes without an explicit stop.
func waitForShutdown(srv interface{ GracefulStop() }) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	stdinClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		close(stdinClosed)
	}()

	select {
	case s := <-sig:
		log.Printf("received signal %v; shutting down", s)
	case <-stdinClosed:
		log.Println("stdin closed (parent exited); shutting down")
	}
	srv.GracefulStop()
}
