package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSafeModFileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		kind     mcmanagerv1.ModKind
		wantName string
		wantErr  bool
	}{
		{name: "plain jar", input: "essentials.jar", kind: mcmanagerv1.ModKind_PLUGIN, wantName: "essentials.jar"},
		{name: "strips slash path", input: "sub/dir/worldedit.jar", kind: mcmanagerv1.ModKind_PLUGIN, wantName: "worldedit.jar"},
		{name: "neutralizes traversal", input: "../../evil.jar", kind: mcmanagerv1.ModKind_MOD, wantName: "evil.jar"},
		{name: "neutralizes backslash traversal", input: `..\..\evil.jar`, kind: mcmanagerv1.ModKind_MOD, wantName: "evil.jar"},
		{name: "allows litemod for mods", input: "armor.litemod", kind: mcmanagerv1.ModKind_MOD, wantName: "armor.litemod"},
		{name: "rejects litemod for plugins", input: "armor.litemod", kind: mcmanagerv1.ModKind_PLUGIN, wantErr: true},
		{name: "rejects non-archive", input: "notes.txt", kind: mcmanagerv1.ModKind_MOD, wantErr: true},
		{name: "rejects bare extension", input: "plugin", kind: mcmanagerv1.ModKind_PLUGIN, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeModFileName(tt.input, tt.kind)
			if tt.wantErr {
				if err == nil {
					t.Errorf("safeModFileName(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeModFileName(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.wantName {
				t.Errorf("safeModFileName(%q) = %q, want %q", tt.input, got, tt.wantName)
			}
		})
	}
}

func TestModLayout(t *testing.T) {
	tests := []struct {
		layout    string
		subdir    string
		kind      mcmanagerv1.ModKind
		supported bool
	}{
		{"plugins", "plugins", mcmanagerv1.ModKind_PLUGIN, true},
		{"mods", "mods", mcmanagerv1.ModKind_MOD, true},
		{"none", "", mcmanagerv1.ModKind_MOD_KIND_UNSPECIFIED, false},
	}
	for _, tt := range tests {
		subdir, kind, ok := modLayout(tt.layout)
		if subdir != tt.subdir || kind != tt.kind || ok != tt.supported {
			t.Errorf("modLayout(%q) = (%q, %v, %v), want (%q, %v, %v)",
				tt.layout, subdir, kind, ok, tt.subdir, tt.kind, tt.supported)
		}
	}
}

func TestModServiceList(t *testing.T) {
	st := store.NewMemory()
	paths := appdata.Paths{Base: t.TempDir()}
	_ = st.Put(&store.Server{ID: "s1", ProviderID: "paper", ModLayout: "plugins", Status: mcmanagerv1.ServerStatus_STOPPED})

	pluginsDir := filepath.Join(paths.ServerDir("s1"), "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, f := range []string{"beta.jar", "alpha.jar", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(pluginsDir, f), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	svc := NewModService(st, paths, nil)
	list, err := svc.List(context.Background(), &mcmanagerv1.ModListRequest{ServerId: "s1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !list.Supported || list.Kind != mcmanagerv1.ModKind_PLUGIN {
		t.Errorf("List supported/kind = %v/%v, want true/PLUGIN", list.Supported, list.Kind)
	}
	got := make([]string, len(list.Files))
	for i, f := range list.Files {
		got[i] = f.Name
	}
	if len(got) != 2 || got[0] != "alpha.jar" || got[1] != "beta.jar" {
		t.Errorf("List files = %v, want sorted [alpha.jar beta.jar] (txt excluded)", got)
	}
}

func TestModServiceListIncludesLiteModForMods(t *testing.T) {
	st := store.NewMemory()
	paths := appdata.Paths{Base: t.TempDir()}
	_ = st.Put(&store.Server{ID: "s1", ProviderID: "forge", ModLayout: "mods", Status: mcmanagerv1.ServerStatus_STOPPED})

	modsDir := filepath.Join(paths.ServerDir("s1"), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"legacy.litemod", "modern.jar", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(modsDir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	svc := NewModService(st, paths, nil)
	list, err := svc.List(context.Background(), &mcmanagerv1.ModListRequest{ServerId: "s1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Files) != 2 || list.Files[0].Name != "legacy.litemod" || list.Files[1].Name != "modern.jar" {
		t.Errorf("files = %v", list.Files)
	}
}

func TestModServiceListPaging(t *testing.T) {
	st := store.NewMemory()
	paths := appdata.Paths{Base: t.TempDir()}
	_ = st.Put(&store.Server{ID: "s1", ProviderID: "paper", ModLayout: "plugins", Status: mcmanagerv1.ServerStatus_STOPPED})
	dir := filepath.Join(paths.ServerDir("s1"), "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"d.jar", "b.jar", "a.jar", "c.jar"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	list, err := NewModService(st, paths, nil).List(context.Background(), &mcmanagerv1.ModListRequest{
		ServerId: "s1", Offset: 1, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 4 || list.Offset != 1 || list.NextOffset != 3 || !list.HasMore {
		t.Fatalf("paging = total:%d offset:%d next:%d more:%v", list.Total, list.Offset, list.NextOffset, list.HasMore)
	}
	if len(list.Files) != 2 || list.Files[0].Name != "b.jar" || list.Files[1].Name != "c.jar" {
		t.Fatalf("files = %v, want [b.jar c.jar]", list.Files)
	}
}

func TestModServiceListUnsupported(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(&store.Server{ID: "v1", ProviderID: "vanilla", ModLayout: "none", Status: mcmanagerv1.ServerStatus_STOPPED})
	svc := NewModService(st, appdata.Paths{Base: t.TempDir()}, nil)

	list, err := svc.List(context.Background(), &mcmanagerv1.ModListRequest{ServerId: "v1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Supported {
		t.Errorf("List(vanilla).Supported = true, want false")
	}
}

func TestModServiceRemoveRejectsRunning(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(&store.Server{ID: "s1", ProviderID: "paper", ModLayout: "plugins", Status: mcmanagerv1.ServerStatus_RUNNING})
	svc := NewModService(st, appdata.Paths{Base: t.TempDir()}, nil)

	_, err := svc.Remove(context.Background(), &mcmanagerv1.RemoveModRequest{ServerId: "s1", Name: "x.jar"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("Remove on running server: code = %v, want FailedPrecondition", status.Code(err))
	}
}
