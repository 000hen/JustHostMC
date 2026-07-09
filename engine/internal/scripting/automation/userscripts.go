package automation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// LoadUserScripts registers every automation script (loose *.lua file) under
// dir into the manager (left disabled). A missing dir is not an error; a single
// bad script is reported via the returned (first) error but does not stop the
// others from loading. The caller decides which scripts to enable.
func LoadUserScripts(ctx context.Context, m *Manager, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var firstErr error
	note := func(e error) {
		if e != nil && firstErr == nil {
			firstErr = e
		}
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".lua") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			note(err)
			continue
		}
		if _, err := m.AddSource(ctx, string(src), false); err != nil {
			note(err)
		}
	}
	return firstErr
}
