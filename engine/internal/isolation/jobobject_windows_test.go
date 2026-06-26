package isolation

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a real test: when JHMC_HELPER=1 it impersonates a
// Minecraft server (prints a ready line, echoes stdin, exits on "stop") so the
// job-object lifecycle can be tested without Java.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("JHMC_HELPER") != "1" {
		return
	}
	fmt.Println("Starting minecraft server version test")
	fmt.Println(`Done (1.234s)! For help, type "help"`)
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := sc.Text()
		if line == "stop" {
			fmt.Println("Stopping the server")
			os.Exit(0)
		}
		fmt.Println("received: " + line)
	}
	os.Exit(0)
}

func helperSpec(id string) InstanceSpec {
	return InstanceSpec{
		ID:       id,
		Dir:      ".",
		JavaPath: os.Args[0],
		Args:     []string{"-test.run=TestHelperProcess"},
		Env:      map[string]string{"JHMC_HELPER": "1"},
	}
}

func waitForLine(t *testing.T, inst Instance, substr string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-inst.Output():
			if !ok {
				t.Fatalf("output closed before seeing %q", substr)
			}
			if strings.Contains(line, substr) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q", substr)
		}
	}
}

func TestJobObjectGracefulLifecycle(t *testing.T) {
	b := NewJobObjectBackend()
	inst, err := b.Start(context.Background(), helperSpec("s1"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitForLine(t, inst, "Done (", 15*time.Second)

	if err := inst.WriteStdin("say hi"); err != nil {
		t.Fatalf("WriteStdin: %v", err)
	}
	waitForLine(t, inst, "received: say hi", 5*time.Second)

	if err := b.Stop(context.Background(), "s1", true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-inst.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("instance did not stop")
	}
	if inst.Running() {
		t.Error("instance still running after graceful stop")
	}
}

func TestJobObjectForceStop(t *testing.T) {
	b := NewJobObjectBackend()
	inst, err := b.Start(context.Background(), helperSpec("s2"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForLine(t, inst, "Done (", 15*time.Second)

	if err := b.Stop(context.Background(), "s2", false); err != nil {
		t.Fatalf("force Stop: %v", err)
	}
	select {
	case <-inst.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("instance did not terminate")
	}
	if inst.Running() {
		t.Error("instance still running after force stop")
	}
}

func TestJobObjectListAndAttach(t *testing.T) {
	b := NewJobObjectBackend()
	inst, err := b.Start(context.Background(), helperSpec("s3"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForLine(t, inst, "Done (", 15*time.Second)
	defer b.Stop(context.Background(), "s3", false)

	list, err := b.List(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v (len %d), err %v; want 1", list, len(list), err)
	}
	got, err := b.Attach(context.Background(), "s3")
	if err != nil || got.ID() != "s3" {
		t.Fatalf("Attach = %v, err %v; want s3", got, err)
	}
}
