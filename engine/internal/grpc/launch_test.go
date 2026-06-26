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

func TestJavaInitialHeapMB(t *testing.T) {
	cases := []struct{ heap, want int }{
		{0, 0},
		{384, 256},
		{820, 410},
		{1639, 819},
	}
	for _, c := range cases {
		if got := javaInitialHeapMB(c.heap); got != c.want {
			t.Errorf("javaInitialHeapMB(%d) = %d, want %d", c.heap, got, c.want)
		}
	}
}

func TestBuildJavaArgs(t *testing.T) {
	args := buildJavaArgs(1024, 21, []string{"-jar", "server.jar", "nogui"}, "-Dfoo=bar -Duser.language=zh")
	if len(args) < 3 || args[0] != "-Xmx820M" || args[1] != "-Xms410M" {
		t.Fatalf("args = %v, want capped -Xmx with lower -Xms first", args)
	}
	requireArgsContain(t, args,
		"-XX:+UseG1GC",
		"-XX:G1RSetUpdatingPauseTimePercent=5",
		"-Duser.language=en",
		"-Duser.country=US",
		"-Dusing.aikars.flags=https://mcflags.emc.gs",
		"-Daikars.new.flags=true",
		"-Dfoo=bar",
		"-Duser.language=zh",
	)
	if lastArgIndex(args, "-Duser.language=en") < lastArgIndex(args, "-Duser.language=zh") {
		t.Fatalf("args = %v, want forced language after custom language", args)
	}
	joined := strings.Join(args, " ")
	if !strings.HasSuffix(joined, "-jar server.jar nogui") {
		t.Errorf("provider args not appended: %v", args)
	}

	capped := buildJavaArgs(2048, 21, []string{"-jar", "server.jar", "nogui"}, "")
	requireArgsContain(t, capped, "-Xmx1639M", "-Xms819M")
	if lastArgIndex(capped, "-Xms1639M") >= 0 {
		t.Fatalf("args = %v, want lower Xms to avoid pre-touching the full capped heap", capped)
	}

	// No memory cap => no heap flags, but keep the locale stable for log parsing.
	none := buildJavaArgs(0, 0, []string{"-jar", "server.jar"}, "")
	requireArgsContain(t, none, "-Duser.language=en", "-Duser.country=US")
	if strings.Contains(strings.Join(none, " "), "-Xmx") || strings.Contains(strings.Join(none, " "), "-Xms") {
		t.Errorf("buildJavaArgs(0,...) = %v, want no heap flags", none)
	}
	if !strings.HasSuffix(strings.Join(none, " "), "-jar server.jar") {
		t.Errorf("buildJavaArgs(0,...) = %v, want provider args appended", none)
	}
}

func TestDefaultJavaArgsUsesLargeHeapTuning(t *testing.T) {
	standard := defaultJavaArgs(largeHeapThresholdMB, 21)
	requireArgsContain(t, standard,
		"-XX:G1NewSizePercent=30",
		"-XX:G1MaxNewSizePercent=40",
		"-XX:G1HeapRegionSize=8M",
		"-XX:G1ReservePercent=20",
		"-XX:InitiatingHeapOccupancyPercent=15",
	)

	large := defaultJavaArgs(largeHeapThresholdMB+1, 21)
	requireArgsContain(t, large,
		"-XX:G1NewSizePercent=40",
		"-XX:G1MaxNewSizePercent=50",
		"-XX:G1HeapRegionSize=16M",
		"-XX:G1ReservePercent=15",
		"-XX:InitiatingHeapOccupancyPercent=20",
	)
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

func requireArgsContain(t *testing.T, args []string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		found := false
		for _, arg := range args {
			if arg == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("args = %v, want %q", args, want)
		}
	}
}

func lastArgIndex(args []string, want string) int {
	for i := len(args) - 1; i >= 0; i-- {
		if args[i] == want {
			return i
		}
	}
	return -1
}
