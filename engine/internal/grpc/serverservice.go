package grpcsvc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/console"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/logging"
	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ServerServiceConfig wires the ServerService to its collaborators.
type ServerServiceConfig struct {
	Store     store.Store
	Changes   store.ChangeSource
	Providers *scripting.Registry
	JRE       *jre.Manager
	Backend   isolation.IsolationBackend
	Paths     appdata.Paths
	Console   *console.Hub
	// CloseLogs, if set, closes all open console-log file handles. RemoveAllData
	// calls it before wiping the logs directory (Windows can't delete open files).
	CloseLogs func()
	// OnExit, if set, is called after a server's instance exits (stop or crash),
	// e.g. to reset the player event bus roster for that server.
	OnExit func(id string)
}

// ServerService implements the ServerService RPCs: it orchestrates the provider
// (download), the JRE manager (runtime), and the isolation backend (run).
type ServerService struct {
	mcmanagerv1.UnimplementedServerServiceServer
	cfg ServerServiceConfig

	mu            sync.Mutex
	instances     map[string]isolation.Instance
	stopping      map[string]bool
	installations map[string]*activeInstallation
}

type activeInstallation struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// NewServerService builds a ServerService.
func NewServerService(cfg ServerServiceConfig) *ServerService {
	return &ServerService{
		cfg:           cfg,
		instances:     make(map[string]isolation.Instance),
		stopping:      make(map[string]bool),
		installations: make(map[string]*activeInstallation),
	}
}

// Reconcile reclaims state after an engine restart (PROMPT 禮10.2): instances the
// backend still reports as alive are re-adopted (re-attaching console + exit
// watch); servers that did not survive are marked STOPPED so the registry never
// shows a phantom running server. With the on-machine JobObject backend, kill-on-
// job-close means nothing survives an engine exit, so those reconcile to STOPPED;
// the Docker backend (M7) genuinely re-adopts running containers here.
func (s *ServerService) Reconcile(ctx context.Context) {
	alive, _ := s.cfg.Backend.List(ctx)
	byID := make(map[string]isolation.Instance, len(alive))
	for _, inst := range alive {
		byID[inst.ID()] = inst
	}

	for _, rec := range s.cfg.Store.List() {
		switch rec.Status {
		case mcmanagerv1.ServerStatus_RUNNING,
			mcmanagerv1.ServerStatus_STARTING,
			mcmanagerv1.ServerStatus_STOPPING:
			if inst, ok := byID[rec.ID]; ok && inst.Running() {
				s.mu.Lock()
				s.instances[rec.ID] = inst
				s.mu.Unlock()
				if s.cfg.Console != nil {
					s.cfg.Console.Register(rec.ID, inst)
				}
				go s.watchExit(rec.ID, inst)
				rec.Status = mcmanagerv1.ServerStatus_RUNNING
			} else {
				rec.Status = mcmanagerv1.ServerStatus_STOPPED
			}
			_ = s.cfg.Store.Put(rec)
		}
	}
}

// List returns all registered servers.
func (s *ServerService) List(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.ServerList, error) {
	records := s.cfg.Store.List()
	out := &mcmanagerv1.ServerList{Servers: make([]*mcmanagerv1.Server, 0, len(records))}
	for _, r := range records {
		out.Servers = append(out.Servers, r.Proto())
	}
	return out, nil
}

// WatchChanges streams one server registry mutation per event. Existing
// servers are intentionally obtained through List after the ready handshake.
func (s *ServerService) WatchChanges(_ *mcmanagerv1.Empty, stream grpc.ServerStreamingServer[mcmanagerv1.ServerChangeEvent]) error {
	if s.cfg.Changes == nil {
		return status.Error(codes.Unavailable, "server change stream unavailable")
	}
	subscription := s.cfg.Changes.Subscribe()
	defer subscription.Cancel()

	if err := stream.Send(&mcmanagerv1.ServerChangeEvent{
		Change: &mcmanagerv1.ServerChangeEvent_Ready{Ready: &mcmanagerv1.Empty{}},
	}); err != nil {
		return err
	}

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case change, ok := <-subscription.Events:
			if !ok {
				return status.Error(codes.ResourceExhausted, "server change subscriber fell behind")
			}
			event := &mcmanagerv1.ServerChangeEvent{}
			switch change.Kind {
			case store.ChangeUpsert:
				event.Change = &mcmanagerv1.ServerChangeEvent_Upsert{Upsert: change.Server.Proto()}
			case store.ChangeDeleted:
				event.Change = &mcmanagerv1.ServerChangeEvent_Deleted{
					Deleted: &mcmanagerv1.ServerId{Id: change.ServerID},
				}
			default:
				continue
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

// Instance returns the live instance for a server id, if one is running. It backs
// the metrics stream, which samples the instance directly.
func (s *ServerService) Instance(id string) (isolation.Instance, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.instances[id]
	return inst, ok
}

// Create provisions a new server, streaming install progress, and registers it
// ready to start.
func (s *ServerService) Create(req *mcmanagerv1.CreateServerRequest, stream grpc.ServerStreamingServer[mcmanagerv1.InstallProgress]) error {
	ctx := stream.Context()

	entry, ok := s.cfg.Providers.Get(req.ProviderId)
	if !ok {
		return errorStatus(codes.Unimplemented, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED,
			fmt.Sprintf("provider %q not installed", req.ProviderId), nil)
	}

	id := genID()
	dir := s.cfg.Paths.ServerDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return status.Errorf(codes.Internal, "create server dir: %v", err)
	}
	cleanup := func() {
		_ = s.cfg.Store.Delete(id)
		_ = os.RemoveAll(dir)
	}

	port := resolvePort(int(req.Port))
	rec := &store.Server{
		ID: id, Name: req.Name, ProviderID: req.ProviderId, ModLayout: entry.Meta.ModLayout,
		McVersion: req.McVersion,
		MemoryMB:  int(req.MemoryMb), Port: port, Status: mcmanagerv1.ServerStatus_INSTALLING,
		SortOrder: s.nextSortOrder(),
	}
	_ = s.cfg.Store.Put(rec)

	installCtx, cancelInstall := context.WithCancel(ctx)
	installation := &activeInstallation{
		cancel: cancelInstall,
		done:   make(chan struct{}),
	}
	s.mu.Lock()
	s.installations[id] = installation
	s.mu.Unlock()
	defer func() {
		cancelInstall()
		s.mu.Lock()
		if s.installations[id] == installation {
			delete(s.installations, id)
		}
		close(installation.done)
		s.mu.Unlock()
	}()

	// Persist install output to a log that outlives a failed install (which wipes
	// the server dir), so the cause is findable later (PROMPT 禮15).
	il := s.openInstallLog(id)
	defer il.Close()
	base := newProgressSender(stream)
	send := func(p provider.Progress) {
		il.record(p)
		base(p)
	}

	spec, err := entry.Provider.Install(installCtx, dir, req.McVersion, send)
	if err != nil {
		// Surface the error in the live log so the user can see what went wrong.
		send(provider.Progress{LogLine: "[error] " + err.Error()})
		il.recordLine("[error] install: " + err.Error())
		cleanup()
		return mapInstallError(err)
	}
	// A modpack provider takes an opaque "packId/versionId" as req.McVersion and
	// resolves the concrete Minecraft version in spec.McVersion; prefer it for
	// Java selection and the stored record.
	resolvedVersion := req.McVersion
	if spec.McVersion != "" {
		resolvedVersion = spec.McVersion
	}
	spec.JavaMajor = maxJavaMajor(spec.JavaMajor, resolvedVersion)
	if _, err := s.cfg.JRE.EnsureJRE(installCtx, spec.JavaMajor, send); err != nil {
		send(provider.Progress{LogLine: "[error] " + err.Error()})
		il.recordLine("[error] jre: " + err.Error())
		cleanup()
		return mapInstallError(err)
	}

	if err := writeEULA(dir); err != nil {
		cleanup()
		return status.Errorf(codes.Internal, "write eula: %v", err)
	}
	if err := writeServerProperties(dir, port); err != nil {
		cleanup()
		return status.Errorf(codes.Internal, "write server.properties: %v", err)
	}

	rec.JavaMajor = spec.JavaMajor
	rec.LaunchArgs = spec.Args
	rec.CustomJavaArgs = req.CustomJavaArgs
	if spec.McVersion != "" {
		rec.McVersion = spec.McVersion
	}
	rec.Loader = spec.Loader
	rec.ProviderVersion = spec.PackVersion
	rec.Status = mcmanagerv1.ServerStatus_STOPPED
	_ = s.cfg.Store.Put(rec)

	send(provider.Progress{Step: "install.progress.done", Fraction: 1})
	return nil
}

// Update changes editable server metadata. Launch settings are allowed only
// while the server is stopped.
func (s *ServerService) Update(ctx context.Context, req *mcmanagerv1.UpdateServerRequest) (*mcmanagerv1.Server, error) {
	rec, ok := s.cfg.Store.Get(req.Id)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "server name is required")
	}
	version := strings.TrimSpace(req.McVersion)
	if version == "" {
		return nil, status.Error(codes.InvalidArgument, "minecraft version is required")
	}
	if req.Port < 0 || req.Port > 65535 {
		return nil, status.Error(codes.InvalidArgument, "port must be between 0 and 65535")
	}
	memoryMB := rec.MemoryMB
	if req.MemoryMb > 0 {
		if req.MemoryMb < 512 || req.MemoryMb > 32768 {
			return nil, status.Error(codes.InvalidArgument, "memory must be between 512 and 32768 MB")
		}
		memoryMB = int(req.MemoryMb)
	}

	port := resolvePort(int(req.Port))
	versionChanged := version != rec.McVersion
	portChanged := port != rec.Port
	memoryChanged := memoryMB != rec.MemoryMB
	customArgsChanged := req.CustomJavaArgs != rec.CustomJavaArgs
	launchChanged := versionChanged || portChanged || memoryChanged || customArgsChanged
	if launchChanged && !isEditableStopped(rec.Status) {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SERVER_RUNNING,
			"stop server before changing launch settings", nil)
	}
	if portChanged && req.Port > 0 && !isPortFree(port) {
		return nil, errorStatus(codes.AlreadyExists, mcmanagerv1.ErrorCode_PORT_IN_USE,
			fmt.Sprintf("port %d is already in use", port), map[string]string{"port": fmt.Sprint(port)})
	}

	dir := s.cfg.Paths.ServerDir(rec.ID)
	if launchChanged {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, status.Errorf(codes.Internal, "create server dir: %v", err)
		}
	}
	if versionChanged {
		entry, ok := s.cfg.Providers.Get(rec.ProviderID)
		if !ok {
			return nil, errorStatus(codes.Unimplemented, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED,
				fmt.Sprintf("provider %q not installed", rec.ProviderID), nil)
		}
		// Hidden providers (e.g. a modpack) install from an opaque version id, so
		// the version picker is meaningless; the app hides it, and this backstops
		// a direct API edit.
		if entry.Meta.Hidden {
			return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED,
				"this server's version is managed by its source and cannot be changed", nil)
		}
		spec, err := entry.Provider.Install(ctx, dir, version, nil)
		if err != nil {
			return nil, mapInstallError(err)
		}
		spec.JavaMajor = maxJavaMajor(spec.JavaMajor, version)
		if s.cfg.JRE != nil {
			if _, err := s.cfg.JRE.EnsureJRE(ctx, spec.JavaMajor, nil); err != nil {
				return nil, mapInstallError(err)
			}
		}
		rec.JavaMajor = spec.JavaMajor
		rec.LaunchArgs = spec.Args
	}
	if portChanged {
		if err := updateServerPropertiesPort(dir, port); err != nil {
			return nil, status.Errorf(codes.Internal, "write server.properties: %v", err)
		}
	}

	rec.Name = name
	rec.McVersion = version
	rec.Port = port
	rec.MemoryMB = memoryMB
	rec.CustomJavaArgs = req.CustomJavaArgs
	rec.SortOrder = int(req.SortOrder)
	if err := s.cfg.Store.Put(rec); err != nil {
		return nil, status.Errorf(codes.Internal, "save server: %v", err)
	}
	return rec.Proto(), nil
}

// UpdateModpack moves a modpack server to another pack version in place,
// streaming progress like Create. Unlike Create, a failure leaves the existing
// install untouched (no cleanup wipe) — the old version keeps working.
func (s *ServerService) UpdateModpack(req *mcmanagerv1.UpdateModpackRequest, stream grpc.ServerStreamingServer[mcmanagerv1.InstallProgress]) error {
	ctx := stream.Context()
	rec, ok := s.cfg.Store.Get(req.Id)
	if !ok {
		return status.Error(codes.NotFound, "server not found")
	}
	if rec.ProviderVersion == "" {
		return errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED,
			"server was not installed from a modpack", nil)
	}
	if rec.Status != mcmanagerv1.ServerStatus_STOPPED {
		return errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SERVER_RUNNING,
			"server must be stopped before updating the modpack", nil)
	}
	newVersion := strings.TrimSpace(req.Version)
	if newVersion == "" {
		return status.Error(codes.InvalidArgument, "version is required")
	}
	entry, ok := s.cfg.Providers.Get(rec.ProviderID)
	if !ok {
		return status.Errorf(codes.Unimplemented, "provider %q not installed", rec.ProviderID)
	}
	up, ok := entry.Provider.(provider.Updater)
	if !ok {
		return status.Error(codes.Unimplemented, "provider does not support update")
	}

	oldVersion := rec.ProviderVersion
	rec.Status = mcmanagerv1.ServerStatus_INSTALLING
	_ = s.cfg.Store.Put(rec)
	restore := func() {
		rec.Status = mcmanagerv1.ServerStatus_STOPPED
		_ = s.cfg.Store.Put(rec)
	}

	il := s.openInstallLog(rec.ID)
	defer il.Close()
	il.recordLine("[update] " + oldVersion + " -> " + newVersion)
	base := newProgressSender(stream)
	send := func(p provider.Progress) {
		il.record(p)
		base(p)
	}

	spec, err := up.Update(ctx, s.cfg.Paths.ServerDir(rec.ID), newVersion, oldVersion, send)
	if err != nil {
		send(provider.Progress{LogLine: "[error] " + err.Error()})
		il.recordLine("[error] update: " + err.Error())
		restore()
		return mapInstallError(err)
	}
	resolved := rec.McVersion
	if spec.McVersion != "" {
		resolved = spec.McVersion
	}
	spec.JavaMajor = maxJavaMajor(spec.JavaMajor, resolved)
	if _, err := s.cfg.JRE.EnsureJRE(ctx, spec.JavaMajor, send); err != nil {
		send(provider.Progress{LogLine: "[error] " + err.Error()})
		il.recordLine("[error] jre: " + err.Error())
		restore()
		return mapInstallError(err)
	}

	rec.JavaMajor = spec.JavaMajor
	if len(spec.Args) > 0 {
		rec.LaunchArgs = spec.Args
	}
	if spec.McVersion != "" {
		rec.McVersion = spec.McVersion
	}
	if spec.Loader != "" {
		rec.Loader = spec.Loader
	}
	if spec.PackVersion != "" {
		rec.ProviderVersion = spec.PackVersion
	}
	rec.Status = mcmanagerv1.ServerStatus_STOPPED
	_ = s.cfg.Store.Put(rec)

	send(provider.Progress{Step: "install.progress.done", Fraction: 1})
	return nil
}

// Start launches a previously created server.
func (s *ServerService) Start(ctx context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error) {
	rec, ok := s.cfg.Store.Get(req.Id)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}

	s.mu.Lock()
	_, running := s.instances[req.Id]
	s.mu.Unlock()
	if running {
		return &mcmanagerv1.Empty{}, nil // idempotent
	}

	javaMajor := maxJavaMajor(rec.JavaMajor, rec.McVersion)
	if javaMajor != rec.JavaMajor {
		rec.JavaMajor = javaMajor
		_ = s.cfg.Store.Put(rec)
	}

	javaPath, err := s.cfg.JRE.EnsureJRE(ctx, javaMajor, nil)
	if err != nil {
		return nil, mapInstallError(err)
	}

	inst, err := s.cfg.Backend.Start(ctx, isolation.InstanceSpec{
		ID:        rec.ID,
		Dir:       s.cfg.Paths.ServerDir(rec.ID),
		JavaPath:  javaPath,
		JavaMajor: javaMajor,
		Args:      buildJavaArgs(rec.MemoryMB, javaMajor, rec.LaunchArgs, rec.CustomJavaArgs),
		MemoryMB:  rec.MemoryMB,
		Port:      rec.Port,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "start server: %v", err)
	}

	s.mu.Lock()
	s.instances[rec.ID] = inst
	delete(s.stopping, rec.ID)
	s.mu.Unlock()

	if s.cfg.Console != nil {
		s.cfg.Console.Register(rec.ID, inst)
	}

	rec.Status = mcmanagerv1.ServerStatus_RUNNING
	_ = s.cfg.Store.Put(rec)

	go s.watchExit(rec.ID, inst)
	return &mcmanagerv1.Empty{}, nil
}

// Stop gracefully stops a running server (save-all, then stop, then force).
func (s *ServerService) Stop(ctx context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error) {
	rec, ok := s.cfg.Store.Get(req.Id)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}

	s.mu.Lock()
	inst := s.instances[req.Id]
	if inst != nil {
		s.stopping[req.Id] = true
	}
	s.mu.Unlock()

	if inst == nil {
		rec.Status = mcmanagerv1.ServerStatus_STOPPED
		_ = s.cfg.Store.Put(rec)
		return &mcmanagerv1.Empty{}, nil
	}

	rec.Status = mcmanagerv1.ServerStatus_STOPPING
	_ = s.cfg.Store.Put(rec)

	// Flush the world before stopping so a safe shutdown is consistent (禮10.4).
	_ = inst.WriteStdin("save-all")
	if err := s.cfg.Backend.Stop(ctx, req.Id, true); err != nil {
		return nil, status.Errorf(codes.Internal, "stop server: %v", err)
	}
	return &mcmanagerv1.Empty{}, nil
}

func (s *ServerService) nextSortOrder() int {
	maxOrder := -1
	for _, rec := range s.cfg.Store.List() {
		if rec.SortOrder > maxOrder {
			maxOrder = rec.SortOrder
		}
	}
	return maxOrder + 1
}

func isEditableStopped(status mcmanagerv1.ServerStatus) bool {
	return status == mcmanagerv1.ServerStatus_STOPPED || status == mcmanagerv1.ServerStatus_CRASHED
}

// Delete stops (force) and removes a server and its data.
func (s *ServerService) Delete(ctx context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error) {
	s.mu.Lock()
	inst := s.instances[req.Id]
	installation := s.installations[req.Id]
	if inst != nil {
		s.stopping[req.Id] = true
	}
	if installation != nil {
		installation.cancel()
	}
	s.mu.Unlock()

	if installation != nil {
		select {
		case <-installation.done:
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
	}

	if inst != nil {
		_ = s.cfg.Backend.Stop(ctx, req.Id, false)
		<-inst.Done()
	}
	if s.cfg.Console != nil {
		s.cfg.Console.Unregister(req.Id)
	}
	_ = s.cfg.Store.Delete(req.Id)
	_ = os.RemoveAll(s.cfg.Paths.ServerDir(req.Id))
	return &mcmanagerv1.Empty{}, nil
}

// RemoveAllData stops every running server and deletes all on-disk data ??server
// folders, backups, logs, and the JRE cache ??and clears the registry, returning
// the engine to a fresh state. It backs the app's "remove all data" action and
// complements a clean uninstall (PROMPT 禮8). The registry database file itself
// lives at the data-dir root (not under a wiped subtree), so its open handle stays
// valid and the engine keeps running afterwards.
func (s *ServerService) RemoveAllData(ctx context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.Empty, error) {
	// Snapshot and force-stop every running instance.
	s.mu.Lock()
	insts := make(map[string]isolation.Instance, len(s.instances))
	for id, inst := range s.instances {
		insts[id] = inst
		s.stopping[id] = true
	}
	s.mu.Unlock()

	for id, inst := range insts {
		_ = s.cfg.Backend.Stop(ctx, id, false)
		select {
		case <-inst.Done():
		case <-time.After(30 * time.Second):
		}
		if s.cfg.Console != nil {
			s.cfg.Console.Unregister(id)
		}
	}

	// Clear the registry rows (keeps the SQLite handle valid).
	for _, rec := range s.cfg.Store.List() {
		_ = s.cfg.Store.Delete(rec.ID)
	}

	// Release open console-log handles before deleting the logs directory.
	if s.cfg.CloseLogs != nil {
		s.cfg.CloseLogs()
	}

	for _, dir := range []string{
		s.cfg.Paths.ServersRoot(),
		s.cfg.Paths.BackupsRoot(),
		s.cfg.Paths.LogsRoot(),
		s.cfg.Paths.JRECache(),
		s.cfg.Paths.ClientAssetsCache(),
	} {
		if err := os.RemoveAll(dir); err != nil {
			return nil, status.Errorf(codes.Internal, "remove %s: %v", dir, err)
		}
	}
	if err := os.MkdirAll(s.cfg.Paths.Base, 0o755); err != nil {
		return nil, status.Errorf(codes.Internal, "recreate base: %v", err)
	}

	s.mu.Lock()
	s.instances = make(map[string]isolation.Instance)
	s.stopping = make(map[string]bool)
	s.mu.Unlock()

	return &mcmanagerv1.Empty{}, nil
}

// watchExit updates persisted status when an instance exits, distinguishing a
// user-requested stop from a crash (groundwork for M3 crash detection).
func (s *ServerService) watchExit(id string, inst isolation.Instance) {
	<-inst.Done()

	s.mu.Lock()
	delete(s.instances, id)
	wasStopping := s.stopping[id]
	delete(s.stopping, id)
	s.mu.Unlock()

	if s.cfg.OnExit != nil {
		s.cfg.OnExit(id)
	}

	rec, ok := s.cfg.Store.Get(id)
	if !ok {
		return
	}
	if wasStopping || inst.ExitErr() == nil {
		rec.Status = mcmanagerv1.ServerStatus_STOPPED
	} else {
		rec.Status = mcmanagerv1.ServerStatus_CRASHED
	}
	_ = s.cfg.Store.Put(rec)
}

// installLog persists install progress to a per-server log file. A nil file
// (open failed) turns every method into a no-op so logging never blocks installs.
type installLog struct{ lg *logging.Logger }

// openInstallLog creates a timestamped install log under the logs root (separate
// from the server dir, so it survives a failed install's cleanup).
func (s *ServerService) openInstallLog(id string) *installLog {
	path := filepath.Join(s.cfg.Paths.LogsRoot(), id, "install-"+time.Now().Format("20060102T150405")+".log")
	lg, err := logging.Open(path)
	if err != nil {
		return &installLog{}
	}
	return &installLog{lg: lg}
}

func (il *installLog) record(p provider.Progress) {
	if il.lg == nil {
		return
	}
	if p.Step != "" {
		_ = il.lg.WriteLine("[step] " + p.Step)
	}
	if p.LogLine != "" {
		_ = il.lg.WriteLine(p.LogLine)
	}
}

func (il *installLog) recordLine(line string) {
	if il.lg != nil {
		_ = il.lg.WriteLine(line)
	}
}

func (il *installLog) Close() {
	if il.lg != nil {
		_ = il.lg.Close()
	}
}

// newProgressSender adapts provider.Progress to the InstallProgress stream,
// throttling pure-fraction updates (~1% steps) while always forwarding step and
// log-line messages. Fraction < 0 tells the frontend to leave the bar unchanged.
func newProgressSender(stream grpc.ServerStreamingServer[mcmanagerv1.InstallProgress]) func(provider.Progress) {
	lastFraction := -1.0
	lastSent := -2.0
	return func(p provider.Progress) {
		if p.Fraction >= 0 {
			lastFraction = p.Fraction
		} else if p.Step != "" {
			lastFraction = -1 // a new step with unknown size => indeterminate
		}

		// Throttle chunk-only progress to avoid flooding the stream.
		if p.Step == "" && p.LogLine == "" {
			if lastFraction < 1.0 && lastFraction < lastSent+0.01 {
				return
			}
		}
		lastSent = lastFraction

		msg := &mcmanagerv1.InstallProgress{Fraction: lastFraction}
		if p.Step != "" {
			msg.Step = &mcmanagerv1.LocalizedMessage{Key: p.Step}
		}
		if p.LogLine != "" {
			msg.LogLine = p.LogLine
		}
		_ = stream.Send(msg)
	}
}
