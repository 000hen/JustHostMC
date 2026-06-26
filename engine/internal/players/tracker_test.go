package players

import (
	"reflect"
	"testing"
)

const logPrefix = "[12:34:56] [Server thread/INFO]: "

func TestRosterApply(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{
			name:  "join and leave",
			lines: []string{logPrefix + "Alice joined the game", logPrefix + "Bob joined the game", logPrefix + "Alice left the game"},
			want:  []string{"Bob"},
		},
		{
			name:  "list reply replaces roster",
			lines: []string{logPrefix + "Zed joined the game", logPrefix + "There are 2 of a max of 20 players online: Alice, Bob"},
			want:  []string{"Alice", "Bob"},
		},
		{
			name:  "empty list clears roster",
			lines: []string{logPrefix + "Alice joined the game", logPrefix + "There are 0 of a max of 20 players online: "},
			want:  []string{},
		},
		{
			name:  "chat line does not match",
			lines: []string{logPrefix + "<Alice> has anyone joined the game today?"},
			want:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRoster()
			for _, line := range tt.lines {
				r.Apply(line)
			}
			if got := r.Names(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Names() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRosterApplyReportsChange(t *testing.T) {
	r := NewRoster()
	if changed := r.Apply(logPrefix + "Alice joined the game"); !changed {
		t.Errorf("Apply(first join) = false, want true")
	}
	if changed := r.Apply(logPrefix + "Alice joined the game"); changed {
		t.Errorf("Apply(duplicate join) = true, want false")
	}
	if changed := r.Apply(logPrefix + "Bob left the game"); changed {
		t.Errorf("Apply(leave of absent player) = true, want false")
	}
}
