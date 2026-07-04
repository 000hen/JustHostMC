package scriptlog

import (
	"path/filepath"
	"testing"

	"github.com/000hen/justhostmc/engine/internal/logging"
)

func TestPersistentLogBufferRestoresSessionsAndUsesSharedPurge(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs", "automation")
	first, err := NewPersistentLogBuffer(0, root)
	if err != nil {
		t.Fatalf("NewPersistentLogBuffer first: %v", err)
	}
	first.Append("auto1", "from first session")
	history, _, cancel := first.Subscribe()
	cancel()
	if len(history) != 1 || !history[0].CurrentSession {
		t.Fatalf("first history = %+v, want one current-session line", history)
	}
	firstSessionID := history[0].SessionID
	first.Close()

	second, err := NewPersistentLogBuffer(0, root)
	if err != nil {
		t.Fatalf("NewPersistentLogBuffer second: %v", err)
	}
	history, _, cancel = second.Subscribe()
	cancel()
	if len(history) != 1 || history[0].CurrentSession {
		t.Fatalf("restored history = %+v, want one previous-session line", history)
	}
	if history[0].SessionID != firstSessionID || history[0].SessionStartedAt.IsZero() {
		t.Fatalf("restored session metadata = %+v", history[0])
	}

	second.Append("auto1", "from second session")
	history, _, cancel = second.Subscribe()
	cancel()
	if len(history) != 2 {
		t.Fatalf("combined history has %d lines, want 2", len(history))
	}
	if history[1].SessionID == firstSessionID || !history[1].CurrentSession {
		t.Fatalf("new line session metadata = %+v", history[1])
	}
	second.Close()

	removed, _, err := logging.Purge(filepath.Dir(root), logging.Policy{ForceAll: true}, history[1].Timestamp)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if removed != 2 {
		t.Fatalf("Purge removed %d files, want both automation session logs", removed)
	}
}
