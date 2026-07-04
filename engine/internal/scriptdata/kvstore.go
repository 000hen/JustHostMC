// Package scriptdata provides per-script persistent key-value storage for
// automation scripts. Each script's data lives in its own JSON file so scripts
// are strictly isolated from one another.
package scriptdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// KVStore persists string key-value pairs per script under
// <dir>/<scriptID>.json. It is safe for concurrent use.
type KVStore struct {
	mu  sync.Mutex
	dir string
}

// NewKVStore returns a store rooted at dir (created lazily on first write).
func NewKVStore(dir string) *KVStore {
	return &KVStore{dir: dir}
}

// Get returns the value for key in the script's store, or ("", false) if the
// key (or the store) does not exist.
func (kv *KVStore) Get(scriptID, key string) (string, bool) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	data := kv.load(scriptID)
	v, ok := data[key]
	return v, ok
}

// Set persists key=value in the script's store.
func (kv *KVStore) Set(scriptID, key, value string) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	data := kv.load(scriptID)
	data[key] = value
	return kv.save(scriptID, data)
}

// Delete removes a key. Deleting an absent key is a no-op.
func (kv *KVStore) Delete(scriptID, key string) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	data := kv.load(scriptID)
	if _, ok := data[key]; !ok {
		return nil
	}
	delete(data, key)
	return kv.save(scriptID, data)
}

// Keys returns all keys in the script's store, sorted.
func (kv *KVStore) Keys(scriptID string) []string {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	data := kv.load(scriptID)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// path maps a script id to its backing file, sanitizing the id so a hostile
// script id cannot escape the store directory.
func (kv *KVStore) path(scriptID string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, scriptID)
	return filepath.Join(kv.dir, safe+".json")
}

func (kv *KVStore) load(scriptID string) map[string]string {
	data := map[string]string{}
	b, err := os.ReadFile(kv.path(scriptID))
	if err != nil {
		return data
	}
	_ = json.Unmarshal(b, &data)
	return data
}

func (kv *KVStore) save(scriptID string, data map[string]string) error {
	if err := os.MkdirAll(kv.dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(kv.path(scriptID), b, 0o644)
}
