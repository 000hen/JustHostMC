package scripting

import (
	"archive/zip"
	"bufio"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/dl"
	"github.com/000hen/justhostmc/engine/internal/provider"
	lua "github.com/yuin/gopher-lua"
)

// newJHMC builds the `jhmc` table exposed to a script, with every function
// bound to this invocation (server dir, granted permissions, progress sink).
func (inv *invocation) newJHMC(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	reg := func(name string, fn lua.LGFunction) { t.RawSetString(name, L.NewFunction(fn)) }

	reg("http_get", inv.httpGet)
	reg("http_json", inv.httpJSON)
	reg("download", inv.download)
	reg("sha256", inv.sha256File)
	reg("unzip", inv.unzip)
	reg("run_jar", inv.runJar)
	reg("resolve_java", inv.resolveJava)
	reg("java_major_for", inv.javaMajorFor)
	reg("json_decode", inv.jsonDecode)
	reg("json_encode", inv.jsonEncode)
	reg("copy_bundled", inv.copyBundled)
	reg("log", func(L *lua.LState) int {
		inv.emit(provider.Progress{LogLine: L.CheckString(1)})
		return 0
	})

	fs := L.NewTable()
	fs.RawSetString("read", L.NewFunction(inv.fsRead))
	fs.RawSetString("write", L.NewFunction(inv.fsWrite))
	fs.RawSetString("exists", L.NewFunction(inv.fsExists))
	fs.RawSetString("glob", L.NewFunction(inv.fsGlob))
	fs.RawSetString("mkdir", L.NewFunction(inv.fsMkdir))
	fs.RawSetString("remove", L.NewFunction(inv.fsRemove))
	t.RawSetString("fs", fs)

	return t
}

// resolvePath joins a script-supplied relative path against the server dir and
// rejects anything that escapes it (zip-slip / path traversal guard).
func (inv *invocation) resolvePath(L *lua.LState, rel string) string {
	if inv.baseDir == "" {
		inv.fail(L, fmt.Errorf("%w: filesystem is unavailable here", ErrPathEscape))
		return ""
	}
	base := filepath.Clean(inv.baseDir)
	full := filepath.Clean(filepath.Join(base, rel))
	r, err := filepath.Rel(base, full)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		inv.fail(L, fmt.Errorf("%w: %s", ErrPathEscape, rel))
		return ""
	}
	return full
}

// -- network ------------------------------------------------------------------

func (inv *invocation) httpGet(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_NETWORK)
	body, err := inv.fetch(L.CheckString(1))
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	L.Push(lua.LString(body))
	return 1
}

func (inv *invocation) httpJSON(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_NETWORK)
	body, err := inv.fetch(L.CheckString(1))
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		inv.fail(L, fmt.Errorf("http_json: %w", err))
		return 0
	}
	L.Push(goToLua(L, v))
	return 1
}

func (inv *invocation) fetch(url string) (string, error) {
	req, err := http.NewRequestWithContext(inv.ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", scriptUserAgent)
	resp, err := inv.host.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	return string(b), err
}

func (inv *invocation) download(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_NETWORK)
	url := L.CheckString(1)
	opts := L.CheckTable(2)
	dest := strField(opts, "dest")
	if dest == "" {
		inv.fail(L, fmt.Errorf("download: opts.dest is required"))
		return 0
	}
	full := inv.resolvePath(L, dest)

	var h hash.Hash
	var want string
	if s := strField(opts, "sha256"); s != "" {
		h, want = sha256.New(), strings.ToLower(s)
	} else if s := strField(opts, "sha1"); s != "" {
		h, want = sha1.New(), strings.ToLower(s)
	}

	sum, _, err := dl.Download(inv.ctx, &inv.host.client, url, full, h, func(done, total int64) {
		frac := -1.0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
		inv.emit(provider.Progress{Fraction: frac})
	})
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	if want != "" && strings.ToLower(sum) != want {
		inv.fail(L, fmt.Errorf("%s: %w (got %s want %s)", dest, provider.ErrChecksumMismatch, sum, want))
		return 0
	}
	L.Push(lua.LString(full))
	return 1
}

// -- filesystem (confined to the server dir) ----------------------------------

func (inv *invocation) requireFS(L *lua.LState) {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER)
}

func (inv *invocation) fsRead(L *lua.LState) int {
	inv.requireFS(L)
	b, err := os.ReadFile(inv.resolvePath(L, L.CheckString(1)))
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	L.Push(lua.LString(b))
	return 1
}

func (inv *invocation) fsWrite(L *lua.LState) int {
	inv.requireFS(L)
	full := inv.resolvePath(L, L.CheckString(1))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		inv.fail(L, err)
		return 0
	}
	if err := os.WriteFile(full, []byte(L.CheckString(2)), 0o644); err != nil {
		inv.fail(L, err)
		return 0
	}
	return 0
}

func (inv *invocation) fsExists(L *lua.LState) int {
	inv.requireFS(L)
	_, err := os.Stat(inv.resolvePath(L, L.CheckString(1)))
	L.Push(lua.LBool(err == nil))
	return 1
}

func (inv *invocation) fsMkdir(L *lua.LState) int {
	inv.requireFS(L)
	if err := os.MkdirAll(inv.resolvePath(L, L.CheckString(1)), 0o755); err != nil {
		inv.fail(L, err)
		return 0
	}
	return 0
}

func (inv *invocation) fsRemove(L *lua.LState) int {
	inv.requireFS(L)
	if err := os.RemoveAll(inv.resolvePath(L, L.CheckString(1))); err != nil {
		inv.fail(L, err)
		return 0
	}
	return 0
}

// fsGlob returns the server-dir-relative paths matching a relative glob pattern.
func (inv *invocation) fsGlob(L *lua.LState) int {
	inv.requireFS(L)
	base := filepath.Clean(inv.baseDir)
	matches, err := filepath.Glob(inv.resolvePath(L, L.CheckString(1)))
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	out := L.NewTable()
	for _, m := range matches {
		if r, err := filepath.Rel(base, m); err == nil {
			out.Append(lua.LString(filepath.ToSlash(r)))
		}
	}
	L.Push(out)
	return 1
}

// -- process / java -----------------------------------------------------------

func (inv *invocation) resolveJava(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_INSTALL)
	path, err := inv.java(int(L.CheckNumber(1)), L.OptBool(2, false))
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	L.Push(lua.LString(path))
	return 1
}

// java resolves a java.exe for the given major; useJDK picks the full JDK
// (needed by build tools that compile, e.g. Spigot BuildTools).
func (inv *invocation) java(major int, useJDK bool) (string, error) {
	resolve := inv.host.jre
	if useJDK {
		resolve = inv.host.jdk
	}
	if resolve == nil {
		return "", fmt.Errorf("no java resolver configured")
	}
	return resolve(inv.ctx, major, func(p provider.Progress) { inv.emit(p) })
}

// runJar runs `java <args...>` in the server dir (or opts.dir within it),
// streaming stdout/stderr to the progress log. Used for installer jars
// (Forge/NeoForge) and build tools (Spigot BuildTools).
func (inv *invocation) runJar(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_INSTALL)
	opts := L.CheckTable(1)

	javaPath, err := inv.java(int(luaNumberField(opts, "java_major")), luaBoolField(opts, "jdk"))
	if err != nil {
		inv.fail(L, err)
		return 0
	}

	var args []string
	if at, ok := opts.RawGetString("args").(*lua.LTable); ok {
		at.ForEach(func(_, v lua.LValue) { args = append(args, lua.LVAsString(v)) })
	}

	dir := inv.baseDir
	if sub := strField(opts, "dir"); sub != "" {
		dir = inv.resolvePath(L, sub)
	}

	cmd := exec.CommandContext(inv.ctx, javaPath, args...)
	cmd.Dir = dir
	if err := inv.streamCmd(cmd); err != nil {
		inv.fail(L, fmt.Errorf("%w: %v", provider.ErrInstallerFailed, err))
		return 0
	}
	return 0
}

func (inv *invocation) streamCmd(cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	pipe := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			inv.emit(provider.Progress{LogLine: sc.Text()})
		}
	}
	go pipe(stdout)
	go pipe(stderr)
	wg.Wait()
	return cmd.Wait()
}

// -- misc ---------------------------------------------------------------------

func (inv *invocation) sha256File(L *lua.LState) int {
	inv.requireFS(L)
	f, err := os.Open(inv.resolvePath(L, L.CheckString(1)))
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		inv.fail(L, err)
		return 0
	}
	L.Push(lua.LString(hex.EncodeToString(h.Sum(nil))))
	return 1
}

func (inv *invocation) javaMajorFor(L *lua.LState) int {
	L.Push(lua.LNumber(provider.JavaMajorForMC(L.CheckString(1))))
	return 1
}

func (inv *invocation) jsonDecode(L *lua.LState) int {
	var v any
	if err := json.Unmarshal([]byte(L.CheckString(1)), &v); err != nil {
		inv.fail(L, err)
		return 0
	}
	L.Push(goToLua(L, v))
	return 1
}

func (inv *invocation) jsonEncode(L *lua.LState) int {
	b, err := json.Marshal(luaToGo(L.CheckAny(1)))
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	L.Push(lua.LString(b))
	return 1
}

// unzip extracts a zip (relative path within the server dir) into destDir
// (also within the server dir), guarding against zip-slip.
func (inv *invocation) unzip(L *lua.LState) int {
	inv.requireFS(L)
	zipPath := inv.resolvePath(L, L.CheckString(1))
	destDir := inv.resolvePath(L, L.CheckString(2))

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	defer zr.Close()

	dest := filepath.Clean(destDir)
	for _, f := range zr.File {
		target := filepath.Clean(filepath.Join(dest, f.Name))
		if r, err := filepath.Rel(dest, target); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			inv.fail(L, fmt.Errorf("%w: %s", ErrPathEscape, f.Name))
			return 0
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				inv.fail(L, err)
				return 0
			}
			continue
		}
		if err := extractZipFile(f, target); err != nil {
			inv.fail(L, err)
			return 0
		}
	}
	return 0
}

func extractZipFile(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(rc, 1<<30))
	return err
}

// copyBundled copies a file shipped alongside the script (e.g. a user-supplied
// jar in the provider's asset dir) into the server dir, returning dest.
func (inv *invocation) copyBundled(L *lua.LState) int {
	inv.requireFS(L)
	name := L.CheckString(1)
	dest := L.CheckString(2)
	if inv.assetDir == "" {
		inv.fail(L, fmt.Errorf("copy_bundled: this provider has no bundled assets"))
		return 0
	}
	if name != filepath.Base(name) {
		inv.fail(L, fmt.Errorf("%w: %s", ErrPathEscape, name))
		return 0
	}
	full := inv.resolvePath(L, dest)
	if err := copyFile(filepath.Join(inv.assetDir, name), full); err != nil {
		inv.fail(L, err)
		return 0
	}
	L.Push(lua.LString(dest))
	return 1
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// luaNumberField reads a numeric field from a table, returning 0 if absent.
func luaNumberField(tbl *lua.LTable, key string) lua.LNumber {
	if n, ok := tbl.RawGetString(key).(lua.LNumber); ok {
		return n
	}
	return 0
}

// luaBoolField reads a boolean field from a table, returning false if absent.
func luaBoolField(tbl *lua.LTable, key string) bool {
	b, _ := tbl.RawGetString(key).(lua.LBool)
	return bool(b)
}
