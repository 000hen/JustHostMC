// Command engine is the JustHostMC backend daemon. The WinUI app launches it as
// a child process, reads the loopback port it prints, and talks to it over an
// authenticated gRPC channel. It is not meant to be run directly by users.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/backup"
	"github.com/000hen/justhostmc/engine/internal/console"
	grpcsvc "github.com/000hen/justhostmc/engine/internal/grpc"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/logging"
	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/000hen/justhostmc/engine/internal/settings"
	"github.com/000hen/justhostmc/engine/internal/store"
)

const (
	// tokenEnvVar carries the per-launch session token supplied by the app.
	tokenEnvVar = "MCMANAGER_TOKEN"
	// portLinePrefix tags the stdout line that reports the chosen port. The app
	// scans stdout for this prefix to learn where to connect.
	portLinePrefix = "MCMANAGER_PORT="
)

func main() {
	// Logs go to stderr so they never collide with the port handshake on stdout.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	token := os.Getenv(tokenEnvVar)
	if token == "" {
		log.Fatalf("%s is required: the app supplies a per-launch session token", tokenEnvVar)
	}

	lis, err := grpcsvc.Listen()
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port

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
	providers := map[mcmanagerv1.ServerType]provider.Provider{
		mcmanagerv1.ServerType_VANILLA:  provider.NewVanilla(),
		mcmanagerv1.ServerType_PAPER:    provider.NewPaper(),
		mcmanagerv1.ServerType_FORGE:    provider.NewForge(jreMgr.EnsureJRE),
		mcmanagerv1.ServerType_NEOFORGE: provider.NewNeoForge(jreMgr.EnsureJRE),
		mcmanagerv1.ServerType_FABRIC:   provider.NewFabric(),
	}
	// Persist every console line to a daily-rotating per-server log file.
	hub := console.NewHub()
	sink := logging.NewSink(paths.LogsRoot())
	defer sink.CloseAll()
	hub.SetLineObserver(sink.Write)

	// Choose the isolation backend: Docker only when the user opted in AND it is
	// available, otherwise the on-machine Job Object backend (PROMPT 禮10.7).
	settingsStore := settings.NewStore(filepath.Join(paths.Base, "settings.json"))
	backend, activeMode := selectBackend(context.Background(), settingsStore)
	log.Printf("isolation backend: %s", activeMode)

	serverService := grpcsvc.NewServerService(grpcsvc.ServerServiceConfig{
		Store:     registry,
		Providers: providers,
		JRE:       jreMgr,
		Backend:   backend,
		Paths:     paths,
		Console:   hub,
		CloseLogs: sink.CloseAll,
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
	})
	// Apply the retention policy at startup, then periodically.
	go runLogJanitor(settingsStore, paths.LogsRoot())

	srv := grpcsvc.NewServer(grpcsvc.Config{
		Token:           token,
		Providers:       providers,
		ServerService:   serverService,
		ConsoleService:  grpcsvc.NewConsoleService(hub),
		BackupService:   backupService,
		SettingsService: settingsService,
		PlayerService:   grpcsvc.NewPlayerService(hub),
		MetricsService:  grpcsvc.NewMetricsService(serverService),
		ModService:      grpcsvc.NewModService(registry, paths),
	})
	log.Printf("engine data dir: %s", paths.Base)

	// Report the port to the parent on stdout's first line, then flush.
	fmt.Printf("%s%d\n", portLinePrefix, port)
	_ = os.Stdout.Sync()
	log.Printf("engine listening on 127.0.0.1:%d", port)

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

// runLogJanitor applies the stored retention policy immediately and then on a
// timer for the life of the process (PROMPT 禮15).
func runLogJanitor(settingsStore *settings.Store, logsRoot string) {
	purge := func() {
		s, err := settingsStore.Load()
		if err != nil {
			log.Printf("log retention: load settings: %v", err)
			return
		}
		removed, freed, err := logging.Purge(logsRoot, s.Policy(), time.Now())
		if err != nil {
			log.Printf("log retention: purge: %v", err)
		}
		if removed > 0 {
			log.Printf("log retention: purged %d files (%d bytes freed)", removed, freed)
		}
	}

	purge()
	ticker := time.NewTicker(logRetentionInterval)
	defer ticker.Stop()
	for range ticker.C {
		purge()
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
