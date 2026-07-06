package grpcsvc

import (
	"regexp"
	"strings"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

var versionConstraint = regexp.MustCompile(`^(>=|<=|>|<|=|~|\^)?\s*([0-9][0-9A-Za-z._+\-]*|\*)$`)

// modCompatibility returns a copy so the metadata cache remains independent of
// server settings. Changing a server's version therefore updates warnings on
// the next List call without re-reading every jar.
func modCompatibility(meta *mcmanagerv1.ModMetadata, providerID, mcVersion string, kind mcmanagerv1.ModKind) *mcmanagerv1.ModMetadata {
	if meta == nil || !meta.Parsed {
		return meta
	}
	out := *meta
	out.Authors = append([]string(nil), meta.Authors...)
	out.Icon = append([]byte(nil), meta.Icon...)
	out.LoaderMismatch = !loaderMatchesServer(meta.Loader, providerID, kind)
	if matches, known := minecraftVersionMatches(mcVersion, meta.GameVersionRequirement); known {
		out.GameVersionMismatch = !matches
	}
	return &out
}

func loaderMatchesServer(loader, providerID string, kind mcmanagerv1.ModKind) bool {
	loader = strings.ToLower(strings.TrimSpace(loader))
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if loader == "" {
		return true // no declaration means no definite mismatch
	}

	if kind == mcmanagerv1.ModKind_PLUGIN {
		switch loader {
		case "bukkit":
			return true // Bukkit plugins are the common Paper/Spigot format.
		case "paper":
			return providerID == "paper"
		default:
			return false
		}
	}

	switch loader {
	case "forge", "forge-legacy":
		return providerID == "forge"
	case "fabric", "quilt", "neoforge", "liteloader":
		return providerID == loader
	default:
		return providerID == loader
	}
}

// minecraftVersionMatches understands the constraint forms emitted by the
// built-in parsers: Fabric/Quilt predicates, Maven ranges used by Forge, exact
// legacy versions, wildcards, and || alternatives. known=false deliberately
// suppresses a warning for syntax we cannot evaluate safely.
func minecraftVersionMatches(version, requirement string) (matches, known bool) {
	version = strings.TrimSpace(version)
	requirement = strings.TrimSpace(requirement)
	if version == "" || requirement == "" {
		return false, false
	}

	unknownAlternative := false
	for _, alternative := range strings.Split(requirement, "||") {
		matched, understood := matchVersionAlternative(version, strings.TrimSpace(alternative))
		if !understood {
			unknownAlternative = true
			continue
		}
		known = true
		if matched {
			return true, true
		}
	}
	if unknownAlternative {
		return false, false
	}
	return false, known
}

func matchVersionAlternative(version, requirement string) (bool, bool) {
	if requirement == "" {
		return false, false
	}
	if strings.HasPrefix(requirement, "[") && strings.HasSuffix(requirement, "]") && !strings.Contains(requirement, ",") {
		exact := strings.TrimSpace(requirement[1 : len(requirement)-1])
		if exact == "" {
			return false, false
		}
		return compareMCVersion(version, exact) == 0, true
	}
	if (requirement[0] == '[' || requirement[0] == '(') && strings.Contains(requirement, ",") {
		return matchMavenRange(version, requirement)
	}

	tokens := strings.Fields(requirement)
	if len(tokens) == 0 {
		return false, false
	}
	for _, token := range tokens {
		matched, known := matchVersionToken(version, token)
		if !known {
			return false, false
		}
		if !matched {
			return false, true
		}
	}
	return true, true
}

func matchMavenRange(version, requirement string) (bool, bool) {
	if len(requirement) < 3 || (requirement[len(requirement)-1] != ']' && requirement[len(requirement)-1] != ')') {
		return false, false
	}
	body := requirement[1 : len(requirement)-1]
	parts := strings.Split(body, ",")
	if len(parts) != 2 {
		return false, false
	}
	lower, upper := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if lower != "" {
		cmp := compareMCVersion(version, lower)
		if cmp < 0 || (cmp == 0 && requirement[0] == '(') {
			return false, true
		}
	}
	if upper != "" {
		cmp := compareMCVersion(version, upper)
		if cmp > 0 || (cmp == 0 && requirement[len(requirement)-1] == ')') {
			return false, true
		}
	}
	return lower != "" || upper != "", lower != "" || upper != ""
}

func matchVersionToken(version, token string) (bool, bool) {
	match := versionConstraint.FindStringSubmatch(strings.TrimSpace(token))
	if match == nil {
		return false, false
	}
	op, target := match[1], match[2]
	if target == "*" {
		return true, true
	}
	if strings.ContainsAny(target, "xX*") {
		prefix := target
		if i := strings.IndexAny(prefix, "xX*"); i >= 0 {
			prefix = strings.TrimSuffix(prefix[:i], ".")
		}
		return version == prefix || strings.HasPrefix(version, prefix+"."), prefix != ""
	}

	cmp := compareMCVersion(version, target)
	switch op {
	case ">=":
		return cmp >= 0, true
	case ">":
		return cmp > 0, true
	case "<=":
		return cmp <= 0, true
	case "<":
		return cmp < 0, true
	case "~":
		parts := parseMCVersion(target)
		if len(parts) < 2 {
			return false, false
		}
		upper := []int{parts[0], parts[1] + 1}
		return cmp >= 0 && compareVersionParts(parseMCVersion(version), upper) < 0, true
	case "^":
		parts := parseMCVersion(target)
		if len(parts) == 0 {
			return false, false
		}
		upper := []int{parts[0] + 1}
		return cmp >= 0 && compareVersionParts(parseMCVersion(version), upper) < 0, true
	default:
		return cmp == 0, true
	}
}

func compareVersionParts(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
