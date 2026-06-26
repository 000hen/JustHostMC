package grpcsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJavaHeapMB(t *testing.T) {
	cases := []struct{ mem, want int }{
		{0, 0},
		{1024, 820},  // 1024 - 204
		{2048, 1639}, // 2048 - 409
		{8192, 7168}, // 8192 - 1024 (headroom capped)
		{512, 384},   // 512 - 128 (headroom floored)
	}
	for _, c := range cases {
		if got := javaHeapMB(c.mem); got != c.want {
			t.Errorf("javaHeapMB(%d) = %d, want %d", c.mem, got, c.want)
		}
	}
}

func TestBuildJavaArgs(t *testing.T) {
	args := buildJavaArgs(1024, 21, []string{"-jar", "server.jar", "nogui"}, "-Dfoo=bar")
	if len(args) < 3 || args[0] != "-Xmx820M" {
		t.Fatalf("args = %v, want -Xmx820M first", args)
	}
	joined := strings.Join(args, " ")
	if !strings.HasSuffix(joined, "-jar server.jar nogui") {
		t.Errorf("provider args not appended: %v", args)
	}

	// No memory cap => no heap flags, just the provider args.
	none := buildJavaArgs(0, 0, []string{"-jar", "server.jar"}, "")
	if len(none) != 2 || none[0] != "-jar" {
		t.Errorf("buildJavaArgs(0,...) = %v, want passthrough", none)
	}
}

func TestMaxJavaMajorKeepsStoredWhenNewerAndUpgradesWhenStale(t *testing.T) {
	if got := maxJavaMajor(21, "26.2"); got != 25 {
		t.Fatalf("maxJavaMajor(21, 26.2) = %d, want 25", got)
	}
	if got := maxJavaMajor(25, "1.20.4"); got != 25 {
		t.Fatalf("maxJavaMajor(25, 1.20.4) = %d, want stored 25", got)
	}
}

func TestWriteEULAAndProperties(t *testing.T) {
	dir := t.TempDir()
	if err := writeEULA(dir); err != nil {
		t.Fatal(err)
	}
	if err := writeServerProperties(dir, 25570); err != nil {
		t.Fatal(err)
	}

	eula, _ := os.ReadFile(filepath.Join(dir, "eula.txt"))
	if !strings.Contains(string(eula), "eula=true") {
		t.Errorf("eula.txt = %q", eula)
	}
	props, _ := os.ReadFile(filepath.Join(dir, "server.properties"))
	if !strings.Contains(string(props), "server-port=25570") {
		t.Errorf("server.properties = %q", props)
	}
}

func TestResolvePort(t *testing.T) {
	if got := resolvePort(25599); got != 25599 {
		t.Errorf("resolvePort(25599) = %d, want passthrough", got)
	}
	got := resolvePort(0)
	if got < defaultPort || got >= defaultPort+portScanLimit {
		t.Errorf("resolvePort(0) = %d, want in [%d,%d)", got, defaultPort, defaultPort+portScanLimit)
	}
	if !isPortFree(got) {
		t.Errorf("resolvePort(0) returned busy port %d", got)
	}
}
