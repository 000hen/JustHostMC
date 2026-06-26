package provider

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// JavaMajorForMC maps a Minecraft version to the Java feature version it needs.
// Mojang metadata is authoritative for Vanilla; modded providers use this
// conservative fallback when their metadata does not expose a Java version.
func JavaMajorForMC(version string) int {
	major, minor, patch, ok := parseMC(version)
	if !ok {
		if snapshotYear(version) >= 26 {
			return 25
		}
		return 21
	}
	if major >= 26 {
		return 25
	}
	if major != 1 {
		return 21
	}
	if minor > 20 || (minor == 20 && patch >= 5) {
		return 21
	}
	if minor >= 17 { // 1.17-1.20.4
		return 17
	}
	return 8
}

func sortMCDesc(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool {
		ai, bi := mcVersionKey(versions[i]), mcVersionKey(versions[j])
		for k := range ai {
			if ai[k] != bi[k] {
				return ai[k] > bi[k]
			}
		}
		return versions[i] > versions[j]
	})
}

func mcVersionKey(v string) [3]int {
	major, minor, patch, ok := parseMC(v)
	if !ok {
		return [3]int{}
	}
	return [3]int{major, minor, patch}
}

func parseMC(v string) (major, minor, patch int, ok bool) {
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, 0, false
	}
	var err error
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, false
	}
	if minor, err = strconv.Atoi(leadingDigits(parts[1])); err != nil {
		return 0, 0, 0, false
	}
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(leadingDigits(parts[2]))
	}
	return major, minor, patch, true
}

func snapshotYear(v string) int {
	if len(v) < 3 || v[2] != 'w' {
		return 0
	}
	year, err := strconv.Atoi(v[:2])
	if err != nil {
		return 0
	}
	return year
}

func leadingDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if !unicode.IsDigit(r) {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}
