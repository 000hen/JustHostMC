package grpcsvc

import (
	"context"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/console"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// instanceSource hands out the live instance for a server id. *ServerService
// implements it.
type instanceSource interface {
	Instance(id string) (isolation.Instance, bool)
}

type serverTypeSource interface {
	ServerType(id string) (mcmanagerv1.ServerType, bool)
}

// MetricsService streams ~1 Hz resource samples for a running server. Backends
// that implement isolation.Sampler supply the data; for others — or a stopped
// server — a zeroed sample is sent so the UI can render an idle state.
type MetricsService struct {
	mcmanagerv1.UnimplementedMetricsServiceServer
	source   instanceSource
	hub      *console.Hub
	types    serverTypeSource
	interval time.Duration
	tpsEvery time.Duration
}

// NewMetricsService builds a MetricsService that pulls live instances from source.
func NewMetricsService(source instanceSource, hub *console.Hub) *MetricsService {
	types, _ := source.(serverTypeSource)
	return &MetricsService{source: source, hub: hub, types: types, interval: time.Second, tpsEvery: 15 * time.Second}
}

// Watch streams a resource sample immediately and then once per interval until
// the client disconnects.
func (s *MetricsService) Watch(req *mcmanagerv1.ServerId, stream grpc.ServerStreamingServer[mcmanagerv1.ResourceSample]) error {
	if req.Id == "" {
		return status.Error(codes.InvalidArgument, "server_id required")
	}

	ctx := stream.Context()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	var lastTPSQuery time.Time

	for {
		if s.shouldQueryTPS(req.Id, lastTPSQuery) {
			lastTPSQuery = time.Now()
			_ = s.hub.Send(req.Id, "tps")
		}
		if err := stream.Send(s.sample(ctx, req.Id)); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// sample builds the current resource sample for a server, falling back to a
// zeroed sample when the server isn't running or its backend can't measure usage.
func (s *MetricsService) sample(ctx context.Context, id string) *mcmanagerv1.ResourceSample {
	out := &mcmanagerv1.ResourceSample{ServerId: id}
	inst, ok := s.source.Instance(id)
	if !ok {
		return out
	}
	sampler, ok := inst.(isolation.Sampler)
	if !ok {
		return out
	}
	stats, ok := sampler.Sample(ctx)
	if !ok {
		return out
	}
	out.CpuPercent = stats.CPUPercent
	out.MemoryBytes = stats.MemoryBytes
	out.MemoryLimitBytes = stats.MemoryLimitBytes
	out.NetRxBytesPerSec = stats.NetRxBytesPerSec
	out.NetTxBytesPerSec = stats.NetTxBytesPerSec
	out.NetworkAvailable = stats.NetworkAvailable
	out.Tps = stats.TPS
	return out
}

func (s *MetricsService) shouldQueryTPS(id string, last time.Time) bool {
	if s.hub == nil || s.types == nil || (!last.IsZero() && time.Since(last) < s.tpsEvery) {
		return false
	}
	typ, ok := s.types.ServerType(id)
	if !ok {
		return false
	}
	return typ == mcmanagerv1.ServerType_PAPER || typ == mcmanagerv1.ServerType_SPIGOT
}
