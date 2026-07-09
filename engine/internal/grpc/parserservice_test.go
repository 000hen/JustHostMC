package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"google.golang.org/grpc/status"
)

const validParser = `
meta = {
  id = "custom-parser",
  name = "Custom Parser",
  version = "0.1.0",
  formats = { "custom.json" },
  permissions = {
    { kind = "fs_server", reason = "read jars" },
    { kind = "network", reason = "enrich from an online registry" },
  },
}
function parse(ctx)
  return nil
end
`

func newTestParserService(t *testing.T) (*ParserService, *scripting.ParserSet, string) {
	t.Helper()
	dir := t.TempDir()
	grants := scripting.NewGrantStore(filepath.Join(dir, "parser-grants.json"))
	ps := scripting.NewParserSet(scripting.NewHost(nil, nil, nil), grants)
	if err := scripting.LoadBuiltinParsers(context.Background(), ps); err != nil {
		t.Fatalf("LoadBuiltinParsers: %v", err)
	}
	parsersDir := filepath.Join(dir, "parsers")
	return NewParserService(ps, grants, parsersDir), ps, parsersDir
}

func TestParserServiceListIncludesBuiltins(t *testing.T) {
	svc, _, _ := newTestParserService(t)
	list, err := svc.List(context.Background(), &mcmanagerv1.Empty{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Parsers) < 6 {
		t.Fatalf("parsers = %d, want >= 6 builtins", len(list.Parsers))
	}
	var fabric *mcmanagerv1.ParserInfo
	for _, p := range list.Parsers {
		if p.Id == "parser-fabric" {
			fabric = p
		}
	}
	if fabric == nil {
		t.Fatal("parser-fabric missing")
	}
	if !fabric.Builtin || len(fabric.Formats) != 1 || fabric.Formats[0] != "fabric.mod.json" {
		t.Errorf("fabric info = %+v", fabric)
	}
	// Built-ins are trusted: declared permissions granted by default.
	if len(fabric.Granted) != 1 || fabric.Granted[0] != mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER {
		t.Errorf("fabric granted = %v", fabric.Granted)
	}
}

func TestParserServiceImportPersistsAndStartsUngranted(t *testing.T) {
	svc, ps, dir := newTestParserService(t)
	info, err := svc.Import(context.Background(), &mcmanagerv1.ImportParserRequest{LuaSource: validParser})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if info.Id != "custom-parser" || info.Builtin {
		t.Errorf("info = %+v", info)
	}
	// User parsers start with no grants until the user consents.
	if len(info.Granted) != 0 {
		t.Errorf("granted = %v, want none", info.Granted)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom-parser.lua")); err != nil {
		t.Errorf("persisted script missing: %v", err)
	}
	if _, ok := ps.Get("custom-parser"); !ok {
		t.Error("parser not registered")
	}
}

func TestParserServiceImportRejectsInvalid(t *testing.T) {
	svc, _, _ := newTestParserService(t)
	if _, err := svc.Import(context.Background(), &mcmanagerv1.ImportParserRequest{LuaSource: "   "}); err == nil {
		t.Error("empty source should be rejected")
	}
	if _, err := svc.Import(context.Background(), &mcmanagerv1.ImportParserRequest{
		LuaSource: `meta = { id = "x", name = "X", permissions = {} }`, // no parse()
	}); err == nil {
		t.Error("script without parse() should be rejected")
	}
}

func TestParserServiceSetPermissionsClampsToDeclared(t *testing.T) {
	svc, ps, _ := newTestParserService(t)
	if _, err := svc.Import(context.Background(), &mcmanagerv1.ImportParserRequest{LuaSource: validParser}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	info, err := svc.SetPermissions(context.Background(), &mcmanagerv1.SetPermissionsRequest{
		Id: "custom-parser",
		Granted: []mcmanagerv1.PermissionKind{
			mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER,
			mcmanagerv1.PermissionKind_PERMISSION_SERVER_CONTROL, // not declared → clamped
		},
	})
	if err != nil {
		t.Fatalf("SetPermissions: %v", err)
	}
	if len(info.Granted) != 1 || info.Granted[0] != mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER {
		t.Errorf("granted = %v, want fs_server only", info.Granted)
	}
	g := ps.EffectiveGrants("custom-parser")
	if !g.Has(mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER) ||
		g.Has(mcmanagerv1.PermissionKind_PERMISSION_SERVER_CONTROL) {
		t.Errorf("effective grants = %v", g)
	}
}

func TestParserServiceRemove(t *testing.T) {
	svc, ps, dir := newTestParserService(t)
	if _, err := svc.Import(context.Background(), &mcmanagerv1.ImportParserRequest{LuaSource: validParser}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, err := svc.Remove(context.Background(), &mcmanagerv1.ProviderRef{Id: "custom-parser"}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := ps.Get("custom-parser"); ok {
		t.Error("parser still registered after Remove")
	}
	if _, err := os.Stat(filepath.Join(dir, "custom-parser.lua")); !os.IsNotExist(err) {
		t.Error("persisted script not deleted")
	}
	// Removing an unknown parser is a no-op, not an error.
	if _, err := svc.Remove(context.Background(), &mcmanagerv1.ProviderRef{Id: "ghost"}); err != nil {
		t.Errorf("Remove ghost: %v", err)
	}
}

func TestParserServiceCannotRemoveBuiltin(t *testing.T) {
	svc, ps, _ := newTestParserService(t)
	_, err := svc.Remove(context.Background(), &mcmanagerv1.ProviderRef{Id: "parser-fabric"})
	if err == nil {
		t.Fatal("removing a built-in must fail")
	}
	if _, ok := status.FromError(err); !ok {
		t.Errorf("err = %v, want gRPC status", err)
	}
	if _, ok := ps.Get("parser-fabric"); !ok {
		t.Error("built-in vanished")
	}
}
