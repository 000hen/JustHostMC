package scripting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scriptdata"
	lua "github.com/yuin/gopher-lua"
)

// runInv loads src into a fresh sandbox bound to inv and calls its global
// check() function, returning the structured error (if any).
func runInv(t *testing.T, inv *invocation, src string) error {
	t.Helper()
	L, err := inv.prepare(src)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer L.Close()
	err = L.CallByParam(lua.P{Fn: L.GetGlobal("check"), NRet: 0, Protect: true})
	if err != nil && inv.lastErr != nil {
		return inv.lastErr
	}
	return err
}

func TestHTTPRequestFullClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Reply", "pong")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "%s|%s|%s", r.Method, r.Header.Get("X-Test"), body)
	}))
	defer srv.Close()

	inv := &invocation{
		ctx:     context.Background(),
		host:    NewHost(nil, nil, nil),
		granted: GrantSet{mcmanagerv1.PermissionKind_PERMISSION_NETWORK: true},
	}
	src := fmt.Sprintf(`
function check()
  local r = jhmc.http{url=%q, method="post", body="hello", headers={["X-Test"]="abc"}}
  assert(r.status == 201, "status "..r.status)
  assert(r.body == "POST|abc|hello", "body "..r.body)
  assert(r.headers["x-reply"] == "pong", "header")
end`, srv.URL)
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestHTTPRequestReturnsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	inv := &invocation{
		ctx:     context.Background(),
		host:    NewHost(nil, nil, nil),
		granted: GrantSet{mcmanagerv1.PermissionKind_PERMISSION_NETWORK: true},
	}
	src := fmt.Sprintf(`
function check()
  local r = jhmc.http{url=%q}
  assert(r.status == 404, "status "..r.status)
end`, srv.URL)
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestHTTPRequestRequiresNetwork(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	err := runInv(t, inv, `function check() jhmc.http{url="http://127.0.0.1:1"} end`)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied", err)
	}
}

func TestHTTPRequestMaxBodyCapsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("a", 1024))
	}))
	defer srv.Close()

	inv := &invocation{
		ctx:     context.Background(),
		host:    NewHost(nil, nil, nil),
		granted: GrantSet{mcmanagerv1.PermissionKind_PERMISSION_NETWORK: true},
	}
	src := fmt.Sprintf(`
function check()
  local r = jhmc.http{url=%q, max_body=100}
  assert(#r.body == 100, "len "..#r.body)
end`, srv.URL)
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestTimeReturnsUnixSeconds(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	// 1.7e9 ≈ Nov 2023; any real clock is after that and before year 2100.
	src := `function check()
  local t = jhmc.time()
  assert(t > 1.7e9 and t < 4.1e9, "time "..t)
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	kv := scriptdata.NewKVStore(t.TempDir())
	inv := &invocation{
		ctx:      context.Background(),
		host:     NewHost(nil, nil, nil),
		kv:       kv,
		scriptID: "s1",
	}
	src := `function check()
  assert(jhmc.store.get("k") == nil, "empty get")
  jhmc.store.set("k", "v")
  assert(jhmc.store.get("k") == "v", "get after set")
  jhmc.store.set("k2", "v2")
  local keys = jhmc.store.keys()
  assert(#keys == 2, "keys "..#keys)
  jhmc.store.delete("k")
  assert(jhmc.store.get("k") == nil, "get after delete")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v, ok := kv.Get("s1", "k2"); !ok || v != "v2" {
		t.Fatalf("persisted value = %q,%v", v, ok)
	}
}

func TestStoreUnavailableWithoutBinding(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	err := runInv(t, inv, `function check() jhmc.store.set("k", "v") end`)
	if err == nil {
		t.Fatal("expected error when no store is bound")
	}
}
