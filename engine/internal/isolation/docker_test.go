package isolation

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestDetectDockerAvailable(t *testing.T) {
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("24.0.7\n"), nil
	}
	a := DetectDocker(context.Background(), run)
	if !a.Available || a.Version != "24.0.7" {
		t.Errorf("got %+v, want Available with version 24.0.7", a)
	}
}

func TestDetectDockerUnavailableWhenDaemonDown(t *testing.T) {
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("error during connect: open //./pipe/dockerDesktop: not found"), errors.New("exit 1")
	}
	a := DetectDocker(context.Background(), run)
	if a.Available {
		t.Errorf("expected unavailable when the daemon is down, got %+v", a)
	}
	if a.Reason == "" {
		t.Error("expected a diagnostic reason")
	}
}

func TestDetectDockerUnavailableWhenNoVersion(t *testing.T) {
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("   \n"), nil // CLI present, daemon returned nothing
	}
	if DetectDocker(context.Background(), run).Available {
		t.Error("expected unavailable when no server version is reported")
	}
}

func TestSelectModeRequiresConsentAndAvailability(t *testing.T) {
	avail := Availability{Available: true}
	down := Availability{Available: false}

	cases := []struct {
		consent bool
		avail   Availability
		want    BackendMode
	}{
		{true, avail, ModeDocker},     // opted in + available -> Docker
		{false, avail, ModeOnMachine}, // available but no consent -> on-machine
		{true, down, ModeOnMachine},   // consent but unavailable -> fall back
		{false, down, ModeOnMachine},  // neither -> on-machine
	}
	for _, c := range cases {
		if got := SelectMode(c.consent, c.avail); got != c.want {
			t.Errorf("SelectMode(consent=%v, available=%v) = %q, want %q",
				c.consent, c.avail.Available, got, c.want)
		}
	}
}

func TestContainerNameRoundTrip(t *testing.T) {
	name := containerName("abc123")
	if name != "jhmc-abc123" {
		t.Errorf("containerName = %q", name)
	}
	if got := idFromContainerName("/" + name); got != "abc123" {
		t.Errorf("idFromContainerName = %q, want abc123", got)
	}
	if got := idFromContainerName("not-ours"); got != "" {
		t.Errorf("idFromContainerName(non-managed) = %q, want empty", got)
	}
}

func TestDockerImageByJavaMajor(t *testing.T) {
	if got := dockerImage(17); got != "eclipse-temurin:17-jre" {
		t.Errorf("dockerImage(17) = %q", got)
	}
	if got := dockerImage(0); got != "eclipse-temurin:21-jre" {
		t.Errorf("dockerImage(0) should default to 21, got %q", got)
	}
}

func TestDockerRunArgs(t *testing.T) {
	args := dockerRunArgs(InstanceSpec{
		ID:        "s1",
		Dir:       `C:\data\s1`,
		JavaMajor: 21,
		Args:      []string{"-Xmx1024M", "-jar", "server.jar", "nogui"},
		MemoryMB:  2048,
		Port:      25565,
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"run", "-d", "-i",
		"--name jhmc-s1",
		"--label " + dockerLabel + "=1",
		`-v C:\data\s1:/data`,
		"-m 2048m",
		"-p 25565:25565",
		"eclipse-temurin:21-jre java -Xmx1024M -jar server.jar nogui",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("run args missing %q\nfull: %s", want, joined)
		}
	}
}

func TestDockerStopArgs(t *testing.T) {
	if got := dockerStopArgs("s1", true); !slices.Equal(got, []string{"stop", "jhmc-s1"}) {
		t.Errorf("graceful stop args = %v", got)
	}
	if got := dockerStopArgs("s1", false); !slices.Equal(got, []string{"kill", "jhmc-s1"}) {
		t.Errorf("force stop args = %v", got)
	}
}

func TestParsePSNames(t *testing.T) {
	out := "jhmc-aaa\njhmc-bbb\nsome-other-container\n"
	got := parsePSNames(out)
	if !slices.Equal(got, []string{"aaa", "bbb"}) {
		t.Errorf("parsePSNames = %v, want [aaa bbb] (managed only)", got)
	}
}

// DockerBackend must satisfy the IsolationBackend contract at compile time.
var _ IsolationBackend = (*DockerBackend)(nil)
