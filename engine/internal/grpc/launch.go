package grpcsvc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/000hen/justhostmc/engine/internal/provider"
)

const (
	defaultPort          = 25565
	portScanLimit        = 100
	largeHeapThresholdMB = 12 * 1024
)

// javaHeapMB computes the JVM -Xmx that fits under the job object's hard memory
// cap, reserving headroom (20%, clamped 128 MiB..1 GiB) for JVM non-heap memory
// so the OS cap isn't tripped and the process killed.
func javaHeapMB(memoryMB int) int {
	if memoryMB <= 0 {
		return 0
	}
	headroom := memoryMB / 5
	if headroom < 128 {
		headroom = 128
	}
	if headroom > 1024 {
		headroom = 1024
	}
	xmx := memoryMB - headroom
	if xmx < 256 {
		xmx = 256
	}
	if xmx > memoryMB {
		xmx = memoryMB
	}
	return xmx
}

func javaInitialHeapMB(heapMB int) int {
	if heapMB <= 0 {
		return 0
	}
	// The server process runs under a hard memory cap; with AlwaysPreTouch,
	// setting Xms to Xmx commits too much heap up front and starves native memory.
	xms := heapMB / 2
	if xms < 256 {
		xms = 256
	}
	if xms > heapMB {
		xms = heapMB
	}
	return xms
}

// buildJavaArgs prepends heap and JVM tuning flags to the provider's program args.
func buildJavaArgs(memoryMB, javaMajor int, providerArgs []string, customJavaArgs string) []string {
	defaults := defaultJavaArgs(javaHeapMB(memoryMB), javaMajor)
	custom := splitJavaArgs(customJavaArgs)
	locale := forcedLogLocaleJavaArgs()

	args := make([]string, 0, len(defaults)+len(custom)+len(locale)+len(providerArgs))
	args = append(args, defaults...)
	args = append(args, custom...)
	args = append(args, locale...)
	return append(args, providerArgs...)
}

func maxJavaMajor(stored int, mcVersion string) int {
	inferred := provider.JavaMajorForMC(mcVersion)
	if inferred > stored {
		return inferred
	}
	return stored
}

func defaultJavaArgs(heapMB, javaMajor int) []string {
	args := make([]string, 0, 20)
	if heapMB > 0 {
		args = append(args,
			fmt.Sprintf("-Xmx%dM", heapMB),
			fmt.Sprintf("-Xms%dM", javaInitialHeapMB(heapMB)),
		)
	}
	if javaMajor < 8 {
		return args
	}

	newSizePercent := 30
	maxNewSizePercent := 40
	heapRegionSize := "8M"
	reservePercent := 20
	initiatingHeapOccupancyPercent := 15
	if heapMB > largeHeapThresholdMB {
		newSizePercent = 40
		maxNewSizePercent = 50
		heapRegionSize = "16M"
		reservePercent = 15
		initiatingHeapOccupancyPercent = 20
	}

	args = append(args,
		"-XX:+UseG1GC",
		"-XX:+ParallelRefProcEnabled",
		"-XX:MaxGCPauseMillis=200",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+DisableExplicitGC",
		"-XX:+AlwaysPreTouch",
		fmt.Sprintf("-XX:G1NewSizePercent=%d", newSizePercent),
		fmt.Sprintf("-XX:G1MaxNewSizePercent=%d", maxNewSizePercent),
		"-XX:G1HeapRegionSize="+heapRegionSize,
		fmt.Sprintf("-XX:G1ReservePercent=%d", reservePercent),
		"-XX:G1HeapWastePercent=5",
		"-XX:G1MixedGCCountTarget=4",
		fmt.Sprintf("-XX:InitiatingHeapOccupancyPercent=%d", initiatingHeapOccupancyPercent),
		"-XX:G1MixedGCLiveThresholdPercent=90",
		"-XX:G1RSetUpdatingPauseTimePercent=5",
		"-XX:SurvivorRatio=32",
		"-XX:+PerfDisableSharedMem",
		"-XX:MaxTenuringThreshold=1",
		"-Dusing.aikars.flags=https://mcflags.emc.gs",
		"-Daikars.new.flags=true",
	)
	return args
}

func forcedLogLocaleJavaArgs() []string {
	return []string{
		"-Duser.language=en",
		"-Duser.country=US",
	}
}

func splitJavaArgs(s string) []string {
	fields := make([]string, 0)
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			if b.Len() > 0 {
				fields = append(fields, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	if b.Len() > 0 {
		fields = append(fields, b.String())
	}
	return fields
}
func writeEULA(dir string) error {
	return os.WriteFile(filepath.Join(dir, "eula.txt"), []byte("eula=true\n"), 0o644)
}

// writeServerProperties writes a minimal server.properties pinning the port; the
// server fills in the remaining defaults on first start.
func writeServerProperties(dir string, port int) error {
	content := fmt.Sprintf("server-port=%d\nquery.port=%d\n", port, port)
	return os.WriteFile(filepath.Join(dir, "server.properties"), []byte(content), 0o644)
}

func updateServerPropertiesPort(dir string, port int) error {
	path := filepath.Join(dir, "server.properties")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeServerProperties(dir, port)
		}
		return err
	}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	seenServerPort := false
	seenQueryPort := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "server-port="):
			lines[i] = fmt.Sprintf("server-port=%d", port)
			seenServerPort = true
		case strings.HasPrefix(trimmed, "query.port="):
			lines[i] = fmt.Sprintf("query.port=%d", port)
			seenQueryPort = true
		}
	}
	if !seenServerPort {
		lines = append(lines, fmt.Sprintf("server-port=%d", port))
	}
	if !seenQueryPort {
		lines = append(lines, fmt.Sprintf("query.port=%d", port))
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// resolvePort returns the requested port (when > 0) or scans for a free one.
func resolvePort(requested int) int {
	if requested > 0 {
		return requested
	}
	for p := defaultPort; p < defaultPort+portScanLimit; p++ {
		if isPortFree(p) {
			return p
		}
	}
	return defaultPort
}

func isPortFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}
