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
	"github.com/000hen/justhostmc/engine/internal/httpcache"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/logging"
	"github.com/000hen/justhostmc/engine/internal/players"
	"github.com/000hen/justhostmc/engine/internal/scriptdata"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/scripting/automation"
	"github.com/000hen/justhostmc/engine/internal/scriptlog"
	"github.com/000hen/justhostmc/engine/internal/settings"
	"github.com/000hen/justhostmc/engine/internal/store"
)

const (
	// pipeEnvVar carries the named-pipe name supplied by the app.
	pipeEnvVar = "MCMANAGER_PIPE"
	// readyLine is printed to stdout once the engine is listening.
	readyLine = "MCMANAGER_READY"
)

// defaultCurseForgeKey is an optional build-time CurseForge API key, injected
// with: -ldflags "-X main.defaultCurseForgeKey=<key>". The repo ships none;
// a user key set in Settings always wins over this default.
var defaultCurseForgeKey string

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Logs go to stderr so they never collide with the ready handshake on stdout.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	pipeName := os.Getenv(pipeEnvVar)
	if pipeName == "" {
		log.Fatalf("[FATAL] %s is required: the app supplies a named-pipe name", pipeEnvVar)
	}

	lis, err := grpcsvc.ListenPipe(pipeName)
	if err != nil {
		log.Fatalf("[FATAL] listen pipe: %v", err)
	}

	paths := appdata.Default()
	if err := os.MkdirAll(paths.Base, 0o755); err != nil {
		log.Fatalf("[FATAL] create data dir: %v", err)
	}
	registry, err := store.OpenSQLite(filepath.Join(paths.Base, "registry.db"))
	if err != nil {
		log.Fatalf("[FATAL] open registry: %v", err)
	}
	defer registry.Close()

	jreMgr := jre.NewManager(paths.JRECache())
	// Server types are now sandboxed Lua provider scripts. Built-ins are embedded
	// in the binary; user-imported scripts are loaded from the data dir below.
	host := scripting.NewHost(nil, jreMgr.EnsureJRE, jreMgr.EnsureJDK)
	grants := scripting.NewGrantStore(filepath.Join(paths.Base, "grants.json"))
	providers := scripting.NewRegistry(host, grants)
	if err := scripting.LoadBuiltins(ctx, providers); err != nil {
		log.Fatalf("[FATAL] load builtin providers: %v", err)
	}
	providersDir := filepath.Join(paths.Base, "providers")
	if err := scripting.LoadUserProviders(ctx, providers, providersDir); err != nil {
		log.Printf("[WARN] load user providers: %v", err)
	}
	// Mod/plugin metadata parsers: sandboxed Lua scripts that read a jar's
	// embedded descriptor (fabric.mod.json, mods.toml, plugin.yml, ...).
	parserGrants := scripting.NewGrantStore(filepath.Join(paths.Base, "parser-grants.json"))
	parsers := scripting.NewParserSet(host, parserGrants)
	if err := scripting.LoadBuiltinParsers(ctx, parsers); err != nil {
		log.Fatalf("[FATAL] load builtin parsers: %v", err)
	}
	parsersDir := filepath.Join(paths.Base, "parsers")
	if err := scripting.LoadUserParsers(ctx, parsers, parsersDir); err != nil {
		log.Printf("[WARN] load user parsers: %v", err)
	}
	// Persist every console line to a daily-rotating per-server log file.
	hub := console.NewHub()
	sink := logging.NewSink(paths.LogsRoot())
	hub.SetLineObserver(sink.Write)

	// Player event bus: structured join/leave events derived from roster state
	// diffs, feeding automation on_join/on_leave hooks.
	eventBus := players.NewEventBus()
	hub.AddLineObserver(eventBus.Feed)

	// Choose the isolation backend: Docker only when the user opted in AND it is
	// available, otherwise the on-machine Job Object backend (PROMPT 禮10.7).
	settingsStore := settings.NewStore(filepath.Join(paths.Base, "settings.json"))
	// Purge before restoring automation history so expired sessions are never
	// replayed into the UI during this run.
	applyLogRetention(settingsStore, paths.LogsRoot(), sink.CloseAll)
	automationLogs, err := scriptlog.NewPersistentLogBuffer(
		0, filepath.Join(paths.LogsRoot(), "automation"))
	if err != nil {
		log.Fatalf("[FATAL] open automation logs: %v", err)
	}
	closeLogs := func() {
		sink.CloseAll()
		automationLogs.Close()
	}
	defer closeLogs()

	backend, activeMode := selectBackend(ctx, settingsStore)
	log.Printf("[INFO] isolation backend: %s", activeMode)

	// Mod shops: sandboxed Lua scripts that browse/search/download mods from
	// online sources (Modrinth, CurseForge, ...). Shop HTTP traffic goes
	// through a disk-backed ETag cache. Keyed sources resolve their API key
	// from the user's settings first, then the baked-in build default.
	host.SetHTTPCache(httpcache.New(paths.HTTPCache(), 0))
	bakedShopKeys := map[string]string{"curseforge": defaultCurseForgeKey}
	shopKey := func(shopID string) string {
		if s, err := settingsStore.Load(); err == nil {
			if k := s.ShopKeys[shopID]; k != "" {
				return k
			}
		}
		return bakedShopKeys[shopID]
	}
	shopGrants := scripting.NewGrantStore(filepath.Join(paths.Base, "shop-grants.json"))
	shops := scripting.NewShopSet(host, shopGrants, shopKey)
	if err := scripting.LoadBuiltinShops(ctx, shops); err != nil {
		log.Fatalf("[FATAL] load builtin shops: %v", err)
	}
	shopsDir := filepath.Join(paths.Base, "shops")
	if err := scripting.LoadUserShops(ctx, shops, shopsDir); err != nil {
		log.Printf("[WARN] load user shops: %v", err)
	}

	serverService := grpcsvc.NewServerService(grpcsvc.ServerServiceConfig{
		Store:     registry,
		Providers: providers,
		JRE:       jreMgr,
		Backend:   backend,
		Paths:     paths,
		Console:   hub,
		CloseLogs: closeLogs,
		OnExit:    eventBus.Reset,
	})
	// Reclaim state from the persisted registry after a restart.
	serverService.Reconcile(ctx)

	backupService := grpcsvc.NewBackupService(grpcsvc.BackupServiceConfig{
		Manager: backup.NewManager(paths.BackupsRoot()),
		Store:   registry,
		Paths:   paths,
		Console: hub,
	})

	settingsService := grpcsvc.NewSettingsService(grpcsvc.SettingsServiceConfig{
		Store:         settingsStore,
		LogsRoot:      paths.LogsRoot(),
		ActiveMode:    string(activeMode),
		CloseLogs:     closeLogs,
		BakedShopKeys: bakedShopKeys,
	})
	// The startup pass ran before history was restored; continue periodically.
	go runLogJanitor(settingsStore, paths.LogsRoot(), closeLogs)

	playerService := grpcsvc.NewPlayerService(hub, registry, paths)

	// Automation scripts drive running servers via the console hub and the
	// server service. They are sandboxed and permission-gated like providers.
	scriptGrants := scripting.NewGrantStore(filepath.Join(paths.Base, "script-grants.json"))
	scriptsEnabled := scripting.NewEnabledStore(filepath.Join(paths.Base, "scripts-enabled.json"))
	scripts := automation.NewManager(automation.ManagerConfig{
		Host:    host,
		Grants:  scriptGrants,
		Console: hub,
		Control: serverService,
		Logs:    automationLogs,
		Query:   &serverQueryAdapter{store: registry},
		Players: &playerManagerAdapter{events: eventBus, players: playerService},
		Events:  eventBus,
		KV:      scriptdata.NewKVStore(filepath.Join(paths.Base, "script-data")),
	})
	scriptsDir := paths.ScriptsRoot()
	if err := automation.LoadUserScripts(ctx, scripts, scriptsDir); err != nil {
		log.Printf("[WARN] load automation scripts: %v", err)
	}
	for _, id := range scriptsEnabled.EnabledIDs() {
		if err := scripts.Enable(id); err != nil {
			log.Printf("[WARN] enable automation %q: %v", id, err)
		}
	}
	defer scripts.Shutdown()

	modService := grpcsvc.NewModService(registry, paths, parsers)
	srv := grpcsvc.NewServer(grpcsvc.Config{
		Providers:       providers,
		ServerService:   serverService,
		ConsoleService:  grpcsvc.NewConsoleService(hub),
		BackupService:   backupService,
		SettingsService: settingsService,
		PlayerService:   playerService,
		MetricsService:  grpcsvc.NewMetricsService(serverService),
		ModService:      modService,
		ConfigService:   grpcsvc.NewConfigService(registry, paths),
		ProviderService: grpcsvc.NewProviderService(providers, grants, providersDir),
		ScriptService:   grpcsvc.NewScriptService(scripts, scriptGrants, scriptsEnabled, scriptsDir),
		ParserService:   grpcsvc.NewParserService(parsers, parserGrants, parsersDir),
		ShopService:     grpcsvc.NewShopService(shops, shopGrants, shopsDir, registry, modService),
	})
	log.Printf("[INFO] engine data dir: %s", paths.Base)

	// Signal readiness to the parent process.
	fmt.Println(readyLine)
	_ = os.Stdout.Sync()
	log.Printf("[INFO] engine listening on pipe: %s", pipeName)

	go waitForShutdown(srv, cancel)

	if err := srv.Serve(lis); err != nil {
		log.Fatalf("[FATAL] serve: %v", err)
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
		log.Printf("[WARN] docker requested but unavailable (%s); using on-machine backend", avail.Reason)
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
		log.Printf("[WARN] log retention: load settings: %v", err)
		return
	}
	if closeLogs != nil {
		closeLogs()
	}
	removed, freed, err := logging.Purge(logsRoot, s.Policy(), time.Now())
	if err != nil {
		log.Printf("[WARN] log retention: purge: %v", err)
	}
	if removed > 0 {
		log.Printf("[INFO] log retention: purged %d files (%d bytes freed)", removed, freed)
	}
}

// waitForShutdown gracefully stops the server when the OS signals termination or
// when the parent process goes away (our stdin reaches EOF). The latter guards
// against leaking the engine if the app crashes without an explicit stop.
func waitForShutdown(srv interface{ GracefulStop() }, cancel context.CancelFunc) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	stdinClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		close(stdinClosed)
	}()

	select {
	case s := <-sig:
		log.Printf("[INFO] received signal %v; shutting down", s)
	case <-stdinClosed:
		log.Println("[INFO] stdin closed (parent exited); shutting down")
	}
	cancel()
	srv.GracefulStop()
}
