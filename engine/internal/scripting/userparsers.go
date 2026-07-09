package scripting

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// LoadUserParsers registers every user-imported parser (loose *.lua file)
// under dir. A missing dir is not an error; a single bad parser is reported
// via the returned (first) error but does not stop the others from loading.
func LoadUserParsers(ctx context.Context, ps *ParserSet, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var firstErr error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".lua") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil {
			_, err = ps.AddSource(ctx, string(src), false)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
