package scripting

import (
	"archive/zip"
	"bufio"
	"context"
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
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/dl"
	"github.com/000hen/justhostmc/engine/internal/httpcache"
	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/BurntSushi/toml"
	lua "github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
)

// newJHMC builds the `jhmc` table exposed to a script, with every function
// bound to this invocation (server dir, granted permissions, progress sink).
func (inv *invocation) newJHMC(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	reg := func(name string, fn lua.LGFunction) { t.RawSetString(name, L.NewFunction(fn)) }

	reg("http", inv.httpRequest)
	reg("http_get", inv.httpGet)
	reg("http_json", inv.httpJSON)
	reg("http_cache", inv.httpCached)
	reg("download", inv.download)
	reg("download_all", inv.downloadAll)
	reg("sha256", inv.sha256File)
	reg("unzip", inv.unzip)
	reg("run_jar", inv.runJar)
	reg("resolve_java", inv.resolveJava)
	reg("java_major_for", inv.javaMajorFor)
	reg("json_decode", inv.jsonDecode)
	reg("json_encode", inv.jsonEncode)
	reg("toml_decode", inv.tomlDecode)
	reg("yaml_decode", inv.yamlDecode)
	reg("zip_read", inv.zipRead)
	reg("zip_entries", inv.zipEntries)
	reg("copy_bundled", inv.copyBundled)
	reg("log", func(L *lua.LState) int {
		inv.emit(provider.Progress{LogLine: L.CheckString(1)})
		return 0
	})
	reg("time", func(L *lua.LState) int {
		L.Push(lua.LNumber(float64(time.Now().UnixMilli()) / 1000.0))
		return 1
	})

	store := L.NewTable()
	store.RawSetString("get", L.NewFunction(inv.storeGet))
	store.RawSetString("set", L.NewFunction(inv.storeSet))
	store.RawSetString("delete", L.NewFunction(inv.storeDelete))
	store.RawSetString("keys", L.NewFunction(inv.storeKeys))
	t.RawSetString("store", store)

	fs := L.NewTable()
	fs.RawSetString("read", L.NewFunction(inv.fsRead))
	fs.RawSetString("write", L.NewFunction(inv.fsWrite))
	fs.RawSetString("exists", L.NewFunction(inv.fsExists))
	fs.RawSetString("glob", L.NewFunction(inv.fsGlob))
	fs.RawSetString("mkdir", L.NewFunction(inv.fsMkdir))
	fs.RawSetString("remove", L.NewFunction(inv.fsRemove))
	t.RawSetString("fs", fs)

	// jhmc.config exposes the script's typed config values. It is the config
	// surface for automation scripts (which have no per-call ctx table); present
	// only when a config map was supplied for this invocation.
	if inv.config != nil {
		cfg := L.NewTable()
		for k, v := range inv.config {
			cfg.RawSetString(k, lua.LString(v))
		}
		t.RawSetString("config", cfg)
	}

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

// httpRequest is jhmc.http(opts): a full HTTP client for scripts. opts fields:
// url (required), method (default "GET"), body, headers (table), timeout
// (seconds, default 30, 0 keeps the invocation deadline), max_body (bytes,
// default 64 MiB). Returns {status=, body=, headers=} — non-2xx responses are
// returned, not raised, so scripts can inspect the status.
func (inv *invocation) httpRequest(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_NETWORK)
	opts := L.CheckTable(1)

	url := strField(opts, "url")
	if url == "" {
		inv.fail(L, fmt.Errorf("http: opts.url is required"))
		return 0
	}
	method := strings.ToUpper(strField(opts, "method"))
	if method == "" {
		method = http.MethodGet
	}
	timeout := float64(luaNumberField(opts, "timeout"))
	if opts.RawGetString("timeout") == lua.LNil {
		timeout = 30
	}
	maxBody := int64(luaNumberField(opts, "max_body"))
	if maxBody <= 0 {
		maxBody = 64 << 20 // 64 MiB, matches http_get/http_json
	}

	ctx := inv.ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout*float64(time.Second)))
		defer cancel()
	}

	var bodyReader io.Reader
	if body := strField(opts, "body"); body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		inv.fail(L, fmt.Errorf("http: %w", err))
		return 0
	}
	req.Header.Set("User-Agent", scriptUserAgent)
	if ht, ok := opts.RawGetString("headers").(*lua.LTable); ok {
		ht.ForEach(func(k, v lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				req.Header.Set(string(ks), lua.LVAsString(v))
			}
		})
	}

	resp, err := inv.host.client.Do(req)
	if err != nil {
		inv.fail(L, fmt.Errorf("%s %s: %w", method, url, err))
		return 0
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		inv.fail(L, fmt.Errorf("%s %s: read body: %w", method, url, err))
		return 0
	}

	result := L.NewTable()
	result.RawSetString("status", lua.LNumber(resp.StatusCode))
	result.RawSetString("body", lua.LString(respBody))
	hdrs := L.NewTable()
	for k, vs := range resp.Header {
		hdrs.RawSetString(strings.ToLower(k), lua.LString(strings.Join(vs, ", ")))
	}
	result.RawSetString("headers", hdrs)
	L.Push(result)
	return 1
}

// httpCached is jhmc.http_cache(opts): a GET through the engine's disk-backed
// ETag cache. opts: url (required), headers (table), ttl (seconds a cached
// entry is served with no network round-trip; default 300, 0 = always
// revalidate with If-None-Match). Returns {status=, body=, headers=, cached=}
// — like jhmc.http, non-2xx responses are returned, not raised.
func (inv *invocation) httpCached(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_NETWORK)
	opts := L.CheckTable(1)

	url := strField(opts, "url")
	if url == "" {
		inv.fail(L, fmt.Errorf("http_cache: opts.url is required"))
		return 0
	}
	ttl := 300 * time.Second
	if tv := opts.RawGetString("ttl"); tv != lua.LNil {
		ttl = time.Duration(float64(lua.LVAsNumber(tv)) * float64(time.Second))
	}
	headers := map[string]string{"User-Agent": scriptUserAgent}
	if ht, ok := opts.RawGetString("headers").(*lua.LTable); ok {
		ht.ForEach(func(k, v lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				headers[string(ks)] = lua.LVAsString(v)
			}
		})
	}

	cache := inv.host.cache
	if cache == nil {
		cache = httpcache.New("", 0) // network-only passthrough
	}
	res, err := cache.Get(inv.ctx, &inv.host.client, url, headers, ttl)
	if err != nil {
		inv.fail(L, err)
		return 0
	}

	result := L.NewTable()
	result.RawSetString("status", lua.LNumber(res.Status))
	result.RawSetString("body", lua.LString(res.Body))
	result.RawSetString("cached", lua.LBool(res.Cached))
	hdrs := L.NewTable()
	for k, vs := range res.Headers {
		hdrs.RawSetString(strings.ToLower(k), lua.LString(strings.Join(vs, ", ")))
	}
	result.RawSetString("headers", hdrs)
	L.Push(result)
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

// batchItem is one download_all entry, fully resolved on the Lua thread so the
// worker goroutines never touch the Lua state.
type batchItem struct {
	dest, full, url string
	resolveURL      string
	resolveHeaders  map[string]string
	newHash         func() hash.Hash // nil = no checksum
	want            string
}

// downloadAll backs jhmc.download_all(items, opts): a parallel batch download
// with per-file completion progress. Items carry either a direct url or a
// resolve endpoint (GET returning {"data": "<real url>"}, the CurseForge
// download-url convention). The first error cancels the batch.
func (inv *invocation) downloadAll(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_NETWORK)
	itemsTbl := L.CheckTable(1)
	concurrency := 6
	if opts, ok := L.Get(2).(*lua.LTable); ok {
		if n, ok := opts.RawGetString("concurrency").(lua.LNumber); ok && int(n) > 0 {
			concurrency = min(int(n), 16)
		}
	}

	var items []batchItem
	var parseErr error
	itemsTbl.ForEach(func(_, v lua.LValue) {
		if parseErr != nil {
			return
		}
		entry, ok := v.(*lua.LTable)
		if !ok {
			parseErr = fmt.Errorf("download_all: item is not a table")
			return
		}
		it := batchItem{dest: strField(entry, "dest"), url: strField(entry, "url")}
		if it.dest == "" {
			parseErr = fmt.Errorf("download_all: item.dest is required")
			return
		}
		it.full = inv.resolvePath(L, it.dest) // raises via fail on escape
		if s := strField(entry, "sha256"); s != "" {
			it.newHash, it.want = sha256.New, strings.ToLower(s)
		} else if s := strField(entry, "sha1"); s != "" {
			it.newHash, it.want = sha1.New, strings.ToLower(s)
		}
		if res, ok := entry.RawGetString("resolve").(*lua.LTable); ok {
			it.resolveURL = strField(res, "url")
			if hdrs, ok := res.RawGetString("headers").(*lua.LTable); ok {
				it.resolveHeaders = map[string]string{}
				hdrs.ForEach(func(k, hv lua.LValue) {
					it.resolveHeaders[k.String()] = hv.String()
				})
			}
		}
		if it.url == "" && it.resolveURL == "" {
			parseErr = fmt.Errorf("download_all: %s has neither url nor resolve.url", it.dest)
			return
		}
		items = append(items, it)
	})
	if parseErr != nil {
		inv.fail(L, parseErr)
		return 0
	}
	if len(items) == 0 {
		return 0
	}

	total := len(items)
	var done atomic.Int64
	// gRPC stream sends are not safe for concurrent use; serialize emission.
	var emitMu sync.Mutex
	emit := func(p provider.Progress) {
		emitMu.Lock()
		inv.emit(p)
		emitMu.Unlock()
	}

	g, ctx := errgroup.WithContext(inv.ctx)
	g.SetLimit(concurrency)
	for _, it := range items {
		g.Go(func() error {
			url := it.url
			if url == "" {
				u, err := resolveDownloadURL(ctx, &inv.host.client, it.resolveURL, it.resolveHeaders)
				if err != nil {
					return fmt.Errorf("%s: %w", it.dest, err)
				}
				url = u
			}
			var h hash.Hash
			if it.newHash != nil {
				h = it.newHash()
			}
			// Per-byte progress is intentionally dropped: fractions from
			// parallel workers would interleave; completion counts are stable.
			sum, _, err := dl.Download(ctx, &inv.host.client, url, it.full, h, nil)
			if err != nil {
				return fmt.Errorf("%s: %w", it.dest, err)
			}
			if it.want != "" && !strings.EqualFold(sum, it.want) {
				return fmt.Errorf("%s: %w (got %s want %s)", it.dest, provider.ErrChecksumMismatch, sum, it.want)
			}
			emit(provider.Progress{Step: "shop.install.downloading",
				Fraction: float64(done.Add(1)) / float64(total), LogLine: path.Base(it.dest)})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		inv.fail(L, err)
		return 0
	}
	return 0
}

// resolveDownloadURL fetches a JSON endpoint whose "data" field is the real
// download URL (the CurseForge /download-url convention).
func resolveDownloadURL(ctx context.Context, client *http.Client, url string, headers map[string]string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", scriptUserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("CurseForge denied downloading (the author disallows third-party downloads)")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	var out struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("resolve %s: %v", url, err)
	}
	if out.Data == "" {
		return "", fmt.Errorf("resolve %s: empty download url", url)
	}
	return out.Data, nil
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

func (inv *invocation) tomlDecode(L *lua.LState) int {
	var v map[string]any
	if err := toml.Unmarshal([]byte(L.CheckString(1)), &v); err != nil {
		inv.fail(L, fmt.Errorf("toml_decode: %w", err))
		return 0
	}
	L.Push(goToLua(L, v))
	return 1
}

func (inv *invocation) yamlDecode(L *lua.LState) int {
	var v any
	if err := yaml.Unmarshal([]byte(L.CheckString(1)), &v); err != nil {
		inv.fail(L, fmt.Errorf("yaml_decode: %w", err))
		return 0
	}
	L.Push(goToLua(L, v))
	return 1
}

// zipReadCap bounds a single entry read via jhmc.zip_read (icons/descriptors
// are tiny; this only guards against a hostile jar).
const zipReadCap = 16 << 20 // 16 MiB

// zipRead returns one entry's bytes from a zip/jar inside the server dir, or
// nil when the entry does not exist.
func (inv *invocation) zipRead(L *lua.LState) int {
	inv.requireFS(L)
	zipPath := inv.resolvePath(L, L.CheckString(1))
	name := L.CheckString(2)

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			inv.fail(L, err)
			return 0
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, zipReadCap))
		if err != nil {
			inv.fail(L, err)
			return 0
		}
		L.Push(lua.LString(b))
		return 1
	}
	L.Push(lua.LNil)
	return 1
}

// zipEntries lists the entry names of a zip/jar inside the server dir.
func (inv *invocation) zipEntries(L *lua.LState) int {
	inv.requireFS(L)
	zipPath := inv.resolvePath(L, L.CheckString(1))
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	defer zr.Close()
	out := L.NewTable()
	for _, f := range zr.File {
		out.Append(lua.LString(f.Name))
	}
	L.Push(out)
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
// (also within the server dir), guarding against zip-slip. An optional third
// arg { prefix = "overrides/" } extracts only entries under prefix, stripping
// it from each destination — used to splat a pack's overrides/ tree into the
// server root.
func (inv *invocation) unzip(L *lua.LState) int {
	inv.requireFS(L)
	zipPath := inv.resolvePath(L, L.CheckString(1))
	destDir := inv.resolvePath(L, L.CheckString(2))

	var prefix string
	if opts, ok := L.Get(3).(*lua.LTable); ok {
		prefix = strField(opts, "prefix")
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		inv.fail(L, err)
		return 0
	}
	defer zr.Close()

	dest := filepath.Clean(destDir)
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if prefix != "" {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			name = name[len(prefix):]
			if name == "" {
				continue // the prefix directory entry itself
			}
		}
		target := filepath.Clean(filepath.Join(dest, name))
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

// -- per-script persistent store (jhmc.store.*) --------------------------------

// requireStore raises a Lua error when no KV store is wired into this
// invocation (e.g. provider installs, which have no script identity).
func (inv *invocation) requireStore(L *lua.LState) {
	if inv.kv == nil || inv.scriptID == "" {
		inv.fail(L, fmt.Errorf("store: persistent storage is unavailable here"))
	}
}

func (inv *invocation) storeGet(L *lua.LState) int {
	inv.requireStore(L)
	v, ok := inv.kv.Get(inv.scriptID, L.CheckString(1))
	if !ok {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(v))
	return 1
}

func (inv *invocation) storeSet(L *lua.LState) int {
	inv.requireStore(L)
	if err := inv.kv.Set(inv.scriptID, L.CheckString(1), L.CheckString(2)); err != nil {
		inv.fail(L, err)
	}
	return 0
}

func (inv *invocation) storeDelete(L *lua.LState) int {
	inv.requireStore(L)
	if err := inv.kv.Delete(inv.scriptID, L.CheckString(1)); err != nil {
		inv.fail(L, err)
	}
	return 0
}

func (inv *invocation) storeKeys(L *lua.LState) int {
	inv.requireStore(L)
	out := L.NewTable()
	for _, k := range inv.kv.Keys(inv.scriptID) {
		out.Append(lua.LString(k))
	}
	L.Push(out)
	return 1
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
