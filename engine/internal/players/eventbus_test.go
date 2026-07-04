package players

import (
	"reflect"
	"testing"
	"time"
)

func collect(t *testing.T, live <-chan PlayerEvent, n int) []PlayerEvent {
	t.Helper()
	var out []PlayerEvent
	deadline := time.After(time.Second)
	for len(out) < n {
		select {
		case ev := <-live:
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("timed out; got %d/%d events: %v", len(out), n, out)
		}
	}
	return out
}

func TestFeedEmitsJoinAndLeave(t *testing.T) {
	eb := NewEventBus()
	live, cancel := eb.Subscribe()
	defer cancel()

	eb.Feed("srv", "[12:00:00] [Server thread/INFO]: Notch joined the game")
	eb.Feed("srv", "[12:00:01] [Server thread/INFO]: Notch left the game")

	got := collect(t, live, 2)
	want := []PlayerEvent{
		{Type: EventJoin, ServerID: "srv", Name: "Notch"},
		{Type: EventLeave, ServerID: "srv", Name: "Notch"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestFeedIgnoresNonRosterLines(t *testing.T) {
	eb := NewEventBus()
	live, cancel := eb.Subscribe()
	defer cancel()

	eb.Feed("srv", "[12:00:00] [Server thread/INFO]: <Notch> hello joined the game friends")
	eb.Feed("srv", "[12:00:00] [Server thread/INFO]: Done (3.2s)! For help, type \"help\"")
	eb.Feed("srv", "[12:00:02] [Server thread/INFO]: Alice joined the game")

	got := collect(t, live, 1)
	if got[0].Name != "Alice" || got[0].Type != EventJoin {
		t.Fatalf("unexpected first event: %v", got[0])
	}
}

func TestListReplyDiffsRoster(t *testing.T) {
	eb := NewEventBus()
	eb.Feed("srv", ": Alice joined the game")
	eb.Feed("srv", ": Bob joined the game")

	live, cancel := eb.Subscribe()
	defer cancel()

	// The "list" reply says only Bob and Carl are online: Alice left, Carl joined.
	eb.Feed("srv", "[x] [Server thread/INFO]: There are 2 of a max of 20 players online: Bob, Carl")

	got := collect(t, live, 2)
	byName := map[string]EventType{}
	for _, ev := range got {
		byName[ev.Name] = ev.Type
	}
	if byName["Carl"] != EventJoin || byName["Alice"] != EventLeave {
		t.Fatalf("events = %v", got)
	}
}

func TestServersAreIndependent(t *testing.T) {
	eb := NewEventBus()
	eb.Feed("a", ": Notch joined the game")
	if got := eb.OnlinePlayers("a"); !reflect.DeepEqual(got, []string{"Notch"}) {
		t.Fatalf("OnlinePlayers(a) = %v", got)
	}
	if got := eb.OnlinePlayers("b"); len(got) != 0 {
		t.Fatalf("OnlinePlayers(b) = %v", got)
	}
}

func TestResetEmitsLeaves(t *testing.T) {
	eb := NewEventBus()
	eb.Feed("srv", ": Alice joined the game")
	live, cancel := eb.Subscribe()
	defer cancel()

	eb.Reset("srv")
	got := collect(t, live, 1)
	if got[0].Type != EventLeave || got[0].Name != "Alice" {
		t.Fatalf("event = %v", got[0])
	}
	if got := eb.OnlinePlayers("srv"); len(got) != 0 {
		t.Fatalf("roster should be empty after Reset, got %v", got)
	}
}

func TestCancelUnsubscribes(t *testing.T) {
	eb := NewEventBus()
	live, cancel := eb.Subscribe()
	cancel()
	if _, ok := <-live; ok {
		t.Fatal("live channel should be closed after cancel")
	}
	// Feeding after cancel must not panic (send on closed channel).
	eb.Feed("srv", ": Alice joined the game")
}
