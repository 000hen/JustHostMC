package isolation

import (
	"regexp"
	"strconv"
	"sync"
	"time"
)

var tpsLineRE = regexp.MustCompile(`(?i)(?:TPS from last 1m, 5m, 15m|TPS):\s*\*?([0-9]+(?:\.[0-9]+)?)`)

type tpsTracker struct {
	mu     sync.Mutex
	value  float64
	seenAt time.Time
}

func (t *tpsTracker) Observe(line string) {
	m := tpsLineRE.FindStringSubmatch(line)
	if len(m) < 2 {
		return
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return
	}
	if v > 20 {
		v = 20
	}
	t.mu.Lock()
	t.value = v
	t.seenAt = time.Now()
	t.mu.Unlock()
}

func (t *tpsTracker) Value() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if time.Since(t.seenAt) > 45*time.Second {
		return 0
	}
	return t.value
}
