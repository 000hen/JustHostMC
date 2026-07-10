package scripting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ConfigStore persists per-script typed config values (key -> value) as a small
// JSON file, one map per script id. It is the config analog of GrantStore.
// Secrets are stored plaintext — the same trust model as settings.json shop keys.
type ConfigStore struct {
	path string
	mu   sync.RWMutex
	data map[string]map[string]string // script id -> key -> value
}

// NewConfigStore loads (or starts) the config file at path.
func NewConfigStore(path string) *ConfigStore {
	cs := &ConfigStore{path: path, data: map[string]map[string]string{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &cs.data)
	}
	return cs
}

func (c *ConfigStore) save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, b, 0o644)
}

// Values returns a copy of the stored key->value overrides for id (an empty map
// when none). The caller may freely mutate the result.
func (c *ConfigStore) Values(id string) map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string, len(c.data[id]))
	for k, v := range c.data[id] {
		out[k] = v
	}
	return out
}

// Set stores one config override and persists. An empty value clears the
// override (and drops the id entirely once it has no values left).
func (c *ConfigStore) Set(id, key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if value == "" {
		m, ok := c.data[id]
		if !ok {
			return nil
		}
		delete(m, key)
		if len(m) == 0 {
			delete(c.data, id)
		}
		return c.save()
	}
	m := c.data[id]
	if m == nil {
		m = map[string]string{}
		c.data[id] = m
	}
	m[key] = value
	return c.save()
}

// Forget drops all stored config for an id (e.g. when a script is removed).
func (c *ConfigStore) Forget(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data[id]; !ok {
		return nil
	}
	delete(c.data, id)
	return c.save()
}

// fallbackConfigReader overlays a lazily-computed default onto a single script's
// config key when that key has no stored value. It lets one script reuse another
// subsystem's setting (e.g. the ftb provider falling back to the shared
// CurseForge shop key) without persisting a copy of the secret.
type fallbackConfigReader struct {
	inner    ConfigReader
	id       string
	key      string
	fallback func() string
}

// NewFallbackConfigReader wraps inner so that Values(id)[key] falls back to
// fallback() whenever the stored value is empty. Every other id and key passes
// through unchanged.
func NewFallbackConfigReader(inner ConfigReader, id, key string, fallback func() string) ConfigReader {
	return fallbackConfigReader{inner: inner, id: id, key: key, fallback: fallback}
}

func (f fallbackConfigReader) Values(id string) map[string]string {
	vals := f.inner.Values(id)
	if id == f.id && vals[f.key] == "" {
		if v := f.fallback(); v != "" {
			if vals == nil {
				vals = map[string]string{}
			}
			vals[f.key] = v
		}
	}
	return vals
}

// EffectiveConfig merges author-declared defaults with stored overrides into the
// final key->value map handed to a script (its ctx.config / jhmc.config). Stored
// values win over declared defaults.
func EffectiveConfig(options []ConfigOption, stored map[string]string) map[string]string {
	out := make(map[string]string, len(options)+len(stored))
	for _, opt := range options {
		if opt.Default != "" {
			out[opt.Key] = opt.Default
		}
	}
	for k, v := range stored {
		out[k] = v
	}
	return out
}
