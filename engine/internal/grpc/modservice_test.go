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

func TestSafeJarName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantErr  bool
	}{
		{name: "plain jar", input: "essentials.jar", wantName: "essentials.jar"},
		{name: "strips slash path", input: "sub/dir/worldedit.jar", wantName: "worldedit.jar"},
		{name: "neutralizes traversal", input: "../../evil.jar", wantName: "evil.jar"},
		{name: "neutralizes backslash traversal", input: `..\..\evil.jar`, wantName: "evil.jar"},
		{name: "rejects non-jar", input: "notes.txt", wantErr: true},
		{name: "rejects bare extension", input: "plugin", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeJarName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("safeJarName(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeJarName(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.wantName {
				t.Errorf("safeJarName(%q) = %q, want %q", tt.input, got, tt.wantName)
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

	svc := NewModService(st, paths)
	list, err := svc.List(context.Background(), &mcmanagerv1.ServerId{Id: "s1"})
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

func TestModServiceListUnsupported(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(&store.Server{ID: "v1", ProviderID: "vanilla", ModLayout: "none", Status: mcmanagerv1.ServerStatus_STOPPED})
	svc := NewModService(st, appdata.Paths{Base: t.TempDir()})

	list, err := svc.List(context.Background(), &mcmanagerv1.ServerId{Id: "v1"})
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
	svc := NewModService(st, appdata.Paths{Base: t.TempDir()})

	_, err := svc.Remove(context.Background(), &mcmanagerv1.RemoveModRequest{ServerId: "s1", Name: "x.jar"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("Remove on running server: code = %v, want FailedPrecondition", status.Code(err))
	}
}
