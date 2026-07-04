package grpcsvc

import (
	"net"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	winio "github.com/Microsoft/go-winio"
	"google.golang.org/grpc"
)

// ListenPipe opens a Windows Named Pipe listener with the given short name.
// The full pipe path is \\.\pipe\<name>.
func ListenPipe(name string) (net.Listener, error) {
	return winio.ListenPipe(`\\.\pipe\`+name, nil)
}

// Config configures the gRPC server. Providers and service fields are optional
// so tests can build a minimal server.
type Config struct {
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
	ParserService   mcmanagerv1.ParserServiceServer
	ScriptService   mcmanagerv1.ScriptServiceServer
}

// NewServer builds a gRPC server with the given services registered.
// No auth interceptors are installed — the named pipe transport provides
// machine-local access control.
func NewServer(cfg Config) *grpc.Server {
	srv := grpc.NewServer()
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
	if cfg.ParserService != nil {
		mcmanagerv1.RegisterParserServiceServer(srv, cfg.ParserService)
	}
	if cfg.ScriptService != nil {
		mcmanagerv1.RegisterScriptServiceServer(srv, cfg.ScriptService)
	}
	return srv
}

// New builds a gRPC server exposing only EngineService (used by tests).
func New() *grpc.Server {
	return NewServer(Config{})
}
