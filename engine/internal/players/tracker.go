// Package players derives a server's online roster by parsing its console output.
// Minecraft servers have no roster API, but they log join/leave events and answer
// the "list" command, so the roster can be reconstructed from the console stream
// the engine already multiplexes.
package players

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Roster is the set of players currently online for one server. It is updated by
// feeding it console lines and is safe for concurrent use.
type Roster struct {
	mu      sync.Mutex
	present map[string]struct{}
}

// NewRoster returns an empty roster.
func NewRoster() *Roster {
	return &Roster{present: make(map[string]struct{})}
}

// Minecraft (vanilla and forks: Paper, Forge, Fabric) logs join/leave as:
//
//	[12:34:56] [Server thread/INFO]: Notch joined the game
//	[12:34:56] [Server thread/INFO]: Notch left the game
//
// and answers "list" with:
//
//	[12:34:56] [Server thread/INFO]: There are 2 of a max of 20 players online: Alice, Bob
//
// The leading ": " and the Minecraft name charset keep these from matching chat
// lines, which are formatted as "...]: <Name> message".
var (
	reJoin = regexp.MustCompile(`: (\w{1,16}) joined the game\b`)
	reLeft = regexp.MustCompile(`: (\w{1,16}) left the game\b`)
	reList = regexp.MustCompile(`There are \d+ .*?players online:\s*(.*)$`)
)

// Apply parses one console line and updates the roster, returning true if the set
// of online players changed.
func (r *Roster) Apply(line string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if m := reJoin.FindStringSubmatch(line); m != nil {
		return r.add(m[1])
	}
	if m := reLeft.FindStringSubmatch(line); m != nil {
		return r.remove(m[1])
	}
	if m := reList.FindStringSubmatch(line); m != nil {
		return r.reset(m[1])
	}
	return false
}

// Names returns the online players in sorted order.
func (r *Roster) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	names := make([]string, 0, len(r.present))
	for n := range r.present {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (r *Roster) add(name string) bool {
	if _, ok := r.present[name]; ok {
		return false
	}
	r.present[name] = struct{}{}
	return true
}

func (r *Roster) remove(name string) bool {
	if _, ok := r.present[name]; !ok {
		return false
	}
	delete(r.present, name)
	return true
}

// reset replaces the roster with the comma-separated names from a "list" reply.
func (r *Roster) reset(csv string) bool {
	next := make(map[string]struct{})
	for _, part := range strings.Split(csv, ",") {
		if name := strings.TrimSpace(part); name != "" {
			next[name] = struct{}{}
		}
	}
	if sameSet(r.present, next) {
		return false
	}
	r.present = next
	return true
}

func sameSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
