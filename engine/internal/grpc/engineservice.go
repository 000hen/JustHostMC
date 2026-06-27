package grpcsvc

import (
	"context"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
)

// EngineService implements the EngineService RPCs: Health (liveness) and
// ListVersions (delegated to the registered providers).
type EngineService struct {
	mcmanagerv1.UnimplementedEngineServiceServer
	Providers *scripting.Registry
}

// Health is a liveness probe; reaching the handler means auth already passed.
func (s *EngineService) Health(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.Empty, error) {
	return &mcmanagerv1.Empty{}, nil
}

// ListVersions returns the installable Minecraft versions for a server type.
func (s *EngineService) ListVersions(ctx context.Context, q *mcmanagerv1.VersionQuery) (*mcmanagerv1.VersionList, error) {
	prov, ok := s.Providers.Provider(q.ProviderId)
	if !ok {
		return &mcmanagerv1.VersionList{}, nil
	}
	versions, err := prov.Versions(ctx)
	if err != nil {
		return nil, mapInstallError(err)
	}
	return &mcmanagerv1.VersionList{Versions: versions}, nil
}
