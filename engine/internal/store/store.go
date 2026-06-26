// Package store is the server registry. M1 ships an in-memory implementation;
// M3 swaps in a SQLite-backed one behind the same Store interface so state
// survives engine restarts.
package store

import (
	"sort"
	"sync"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// Server is a registered server plus the launch details the engine needs to
// start it. It carries proto enums directly to avoid an extra mapping layer.
type Server struct {
	ID             string
	Name           string
	Type           mcmanagerv1.ServerType
	McVersion      string
	MemoryMB       int
	Port           int
	Status         mcmanagerv1.ServerStatus
	SortOrder      int
	JavaMajor      int
	LaunchArgs     []string
	CustomJavaArgs string
}

// Proto projects a Server onto the wire type returned to the frontend.
func (s *Server) Proto() *mcmanagerv1.Server {
	return &mcmanagerv1.Server{
		Id:             s.ID,
		Name:           s.Name,
		Type:           s.Type,
		McVersion:      s.McVersion,
		MemoryMb:       int32(s.MemoryMB),
		Port:           int32(s.Port),
		Status:         s.Status,
		SortOrder:      int32(s.SortOrder),
		CustomJavaArgs: s.CustomJavaArgs,
	}
}

// Store persists server registry entries.
type Store interface {
	Put(s *Server) error
	Get(id string) (*Server, bool)
	List() []*Server
	Delete(id string) error
}

// Memory is a thread-safe in-memory Store.
type Memory struct {
	mu      sync.RWMutex
	servers map[string]*Server
}

// NewMemory creates an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{servers: make(map[string]*Server)}
}

func (m *Memory) Put(s *Server) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := *s
	m.servers[s.ID] = &clone
	return nil
}

func (m *Memory) Get(id string) (*Server, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.servers[id]
	if !ok {
		return nil, false
	}
	clone := *s
	return &clone, true
}

func (m *Memory) List() []*Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Server, 0, len(m.servers))
	for _, s := range m.servers {
		clone := *s
		out = append(out, &clone)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (m *Memory) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.servers, id)
	return nil
}
