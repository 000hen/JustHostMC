package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSinkWritesPerServerFile(t *testing.T) {
	root := t.TempDir()
	s := NewSink(root)
	s.Write("srv1", "hello")
	s.Write("srv1", "world")
	s.CloseAll()

	day := time.Now().Format("2006-01-02")
	b, err := os.ReadFile(filepath.Join(root, "srv1", "console-"+day+".log"))
	if err != nil {
		t.Fatalf("read console log: %v", err)
	}
	if got, want := string(b), "hello\nworld\n"; got != want {
		t.Errorf("console log = %q, want %q", got, want)
	}
}

func TestSinkRotatesDaily(t *testing.T) {
	root := t.TempDir()
	s := NewSink(root)

	day1 := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)

	s.now = func() time.Time { return day1 }
	s.Write("srv1", "first day")
	s.now = func() time.Time { return day2 }
	s.Write("srv1", "second day")
	s.CloseAll()

	for _, tc := range []struct{ day, want string }{
		{"2026-06-24", "first day\n"},
		{"2026-06-25", "second day\n"},
	} {
		b, err := os.ReadFile(filepath.Join(root, "srv1", "console-"+tc.day+".log"))
		if err != nil {
			t.Fatalf("read %s: %v", tc.day, err)
		}
		if string(b) != tc.want {
			t.Errorf("%s log = %q, want %q", tc.day, string(b), tc.want)
		}
	}
}

func TestSinkCloseThenWriteAppends(t *testing.T) {
	root := t.TempDir()
	s := NewSink(root)
	s.Write("srv1", "one")
	s.Close("srv1")
	s.Write("srv1", "two") // must reopen and append, not truncate
	s.CloseAll()

	day := time.Now().Format("2006-01-02")
	b, err := os.ReadFile(filepath.Join(root, "srv1", "console-"+day+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "one\ntwo\n"; got != want {
		t.Errorf("console log = %q, want %q", got, want)
	}
}
