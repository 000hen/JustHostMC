package console

import (
	"sync"
	"testing"
	"time"
)

// fakeInstance is a controllable isolation.Instance for hub tests.
type fakeInstance struct {
	out  chan string
	done chan struct{}

	mu      sync.Mutex
	written []string
}

func newFakeInstance() *fakeInstance {
	return &fakeInstance{out: make(chan string), done: make(chan struct{})}
}

func (f *fakeInstance) ID() string            { return "fake" }
func (f *fakeInstance) Output() <-chan string { return f.out }
func (f *fakeInstance) Done() <-chan struct{} { return f.done }
func (f *fakeInstance) ExitErr() error        { return nil }
func (f *fakeInstance) WriteStdin(line string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, line)
	return nil
}

func (f *fakeInstance) Running() bool {
	select {
	case <-f.done:
		return false
	default:
		return true
	}
}

func recv(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for console line")
		return ""
	}
}

func TestHubLiveFanOutAndHistoryReplay(t *testing.T) {
	h := NewHub()
	fake := newFakeInstance()
	h.Register("s1", fake)

	_, live1, cancel1 := h.Subscribe("s1")
	defer cancel1()

	fake.out <- "alpha"
	fake.out <- "beta"

	if got := recv(t, live1); got != "alpha" {
		t.Fatalf("live1[0] = %q, want alpha", got)
	}
	if got := recv(t, live1); got != "beta" {
		t.Fatalf("live1[1] = %q, want beta", got)
	}

	// A late subscriber replays history (broadcast appends to ring before send,
	// so once live1 received both, the ring already contains them).
	history, live2, cancel2 := h.Subscribe("s1")
	defer cancel2()
	if len(history) != 2 || history[0] != "alpha" || history[1] != "beta" {
		t.Fatalf("history = %v, want [alpha beta]", history)
	}

	fake.out <- "gamma"
	if got := recv(t, live1); got != "gamma" {
		t.Errorf("live1 gamma = %q", got)
	}
	if got := recv(t, live2); got != "gamma" {
		t.Errorf("live2 gamma = %q", got)
	}
}

func TestHubLineObserverSeesEveryLine(t *testing.T) {
	h := NewHub()

	var mu sync.Mutex
	var seen []string
	h.SetLineObserver(func(id, line string) {
		mu.Lock()
		seen = append(seen, id+":"+line)
		mu.Unlock()
	})

	fake := newFakeInstance()
	h.Register("s1", fake)
	_, live, cancel := h.Subscribe("s1")
	defer cancel()

	fake.out <- "alpha"
	recv(t, live) // ensure the line has been processed

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "s1:alpha" {
		t.Errorf("observer saw %v, want [s1:alpha]", seen)
	}
}

func TestHubSendWritesToStdin(t *testing.T) {
	h := NewHub()
	fake := newFakeInstance()
	h.Register("s1", fake)

	if err := h.Send("s1", "say hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.written) != 1 || fake.written[0] != "say hi" {
		t.Errorf("written = %v, want [say hi]", fake.written)
	}
}

func TestHubSendUnknownServer(t *testing.T) {
	h := NewHub()
	if err := h.Send("nope", "x"); err == nil {
		t.Fatal("expected error for unknown server")
	}
}

func TestHubUnregisterClosesSubscribers(t *testing.T) {
	h := NewHub()
	fake := newFakeInstance()
	h.Register("s1", fake)
	_, live, _ := h.Subscribe("s1")

	h.Unregister("s1")
	select {
	case _, ok := <-live:
		if ok {
			t.Error("expected channel closed after Unregister")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after Unregister")
	}
}
