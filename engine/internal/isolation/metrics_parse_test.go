package isolation

import "testing"

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"0B", 0},
		{"512B", 512},
		{"1kB", 1000},
		{"1KiB", 1024},
		{"1.5MiB", 1572864},
		{"2GiB", 2147483648},
		{"3.4MB", 3400000},
		{"", 0},
		{"garbage", 0},
	}
	for _, tt := range tests {
		if got := parseSize(tt.in); got != tt.want {
			t.Errorf("parseSize(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseUsagePair(t *testing.T) {
	left, right := parseUsagePair("512MiB / 2GiB")
	if left != 536870912 || right != 2147483648 {
		t.Errorf("parseUsagePair(mem) = %d/%d, want 536870912/2147483648", left, right)
	}

	rx, tx := parseUsagePair("1.2kB / 3.4kB")
	if rx != 1200 || tx != 3400 {
		t.Errorf("parseUsagePair(net) = %d/%d, want 1200/3400", rx, tx)
	}

	if l, r := parseUsagePair("no-slash"); l != 0 || r != 0 {
		t.Errorf("parseUsagePair(no slash) = %d/%d, want 0/0", l, r)
	}
}

func TestParsePercent(t *testing.T) {
	if got := parsePercent("12.34%"); got != 12.34 {
		t.Errorf("parsePercent(%q) = %v, want 12.34", "12.34%", got)
	}
	if got := parsePercent("0.00%"); got != 0 {
		t.Errorf("parsePercent(%q) = %v, want 0", "0.00%", got)
	}
}
