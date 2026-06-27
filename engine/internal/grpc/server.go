package grpcsvc

import (
	"net"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"google.golang.org/grpc"
)

// loopbackAddr binds only the loopback interface with an OS-assigned port, so
// the engine is never reachable from off-machine.
const loopbackAddr = "127.0.0.1:0"

// Listen opens a TCP listener on the loopback interface with a random port.
func Listen() (net.Listener, error) {
	return net.Listen("tcp", loopbackAddr)
}

// Config configures the gRPC server. Providers and ServerService are optional so
// the auth and health tests can build a minimal server.
type Config struct {
	Token           string
	Providers       *scripting.Registry
	ServerService   mcmanagerv1.ServerServiceServer
	ConsoleService  mcmanagerv1.ConsoleServiceServer
	BackupService   mcmanagerv1.BackupServiceServer
	SettingsService mcmanagerv1.SettingsServiceServer
	PlayerService   mcmanagerv1.PlayerServiceServer
	MetricsService  mcmanagerv1.MetricsServiceServer
	ModService      mcmanagerv1.ModServiceServer
	ConfigService   mcmanagerv1.ConfigServiceServer
	ProviderService mcmanagerv1.ProviderServiceServer
	ScriptService   mcmanagerv1.ScriptServiceServer
}

// NewServer builds a gRPC server with session-token auth interceptors installed
// and the given services registered.
func NewServer(cfg Config) *grpc.Server {
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(NewUnaryAuthInterceptor(cfg.Token)),
		grpc.ChainStreamInterceptor(NewStreamAuthInterceptor(cfg.Token)),
	)
	mcmanagerv1.RegisterEngineServiceServer(srv, &EngineService{Providers: cfg.Providers})
	if cfg.ServerService != nil {
		mcmanagerv1.RegisterServerServiceServer(srv, cfg.ServerService)
	}
	if cfg.ConsoleService != nil {
		mcmanagerv1.RegisterConsoleServiceServer(srv, cfg.ConsoleService)
	}
	if cfg.BackupService != nil {
		mcmanagerv1.RegisterBackupServiceServer(srv, cfg.BackupService)
	}
	if cfg.SettingsService != nil {
		mcmanagerv1.RegisterSettingsServiceServer(srv, cfg.SettingsService)
	}
	if cfg.PlayerService != nil {
		mcmanagerv1.RegisterPlayerServiceServer(srv, cfg.PlayerService)
	}
	if cfg.MetricsService != nil {
		mcmanagerv1.RegisterMetricsServiceServer(srv, cfg.MetricsService)
	}
	if cfg.ModService != nil {
		mcmanagerv1.RegisterModServiceServer(srv, cfg.ModService)
	}
	if cfg.ConfigService != nil {
		mcmanagerv1.RegisterConfigServiceServer(srv, cfg.ConfigService)
	}
	if cfg.ProviderService != nil {
		mcmanagerv1.RegisterProviderServiceServer(srv, cfg.ProviderService)
	}
	if cfg.ScriptService != nil {
		mcmanagerv1.RegisterScriptServiceServer(srv, cfg.ScriptService)
	}
	return srv
}

// New builds a gRPC server exposing only EngineService (used by tests).
func New(token string) *grpc.Server {
	return NewServer(Config{Token: token})
}
