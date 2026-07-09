package scripting

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// LoadUserProviders registers every user-imported provider under dir. Each
// provider is either a subdirectory containing provider.lua (which may also hold
// a bundled jar) or a loose *.lua file. A missing dir is not an error; a single
// bad script is logged by the caller via the returned (first) error but does not
// stop the others from loading.
func LoadUserProviders(ctx context.Context, r *Registry, dir string) error {
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
		switch {
		case e.IsDir():
			if _, err := os.Stat(filepath.Join(dir, e.Name(), "provider.lua")); err == nil {
				_, err := r.AddProviderDir(ctx, filepath.Join(dir, e.Name()), false)
				note(err)
			}
		case strings.HasSuffix(strings.ToLower(e.Name()), ".lua"):
			src, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				note(err)
				continue
			}
			_, err = r.AddSource(ctx, string(src), false)
			note(err)
		}
	}
	return firstErr
}
