package selfupdate

import (
	"strconv"
	"strings"
)

// CompareVersions compares two semver strings (with optional "v" prefix).
// Returns -1 if current < latest, 0 if equal, 1 if current > latest.
// "dev" prefix versions are always treated as older (-1).
func CompareVersions(current, latest string) int {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Strip build metadata per semver — +dirty, +build, etc. ignored for precedence.
	current, _, _ = strings.Cut(current, "+")
	latest, _, _ = strings.Cut(latest, "+")

	if strings.HasPrefix(current, "dev") {
		return -1
	}

	partsA := strings.Split(current, ".")
	partsB := strings.Split(latest, ".")

	length := len(partsA)
	if len(partsB) > length {
		length = len(partsB)
	}

	for i := range length {
		a := segmentInt(partsA, i)
		b := segmentInt(partsB, i)
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
	}
	return 0
}

// segmentInt returns the integer at index i in parts, or 0 if out of bounds
// or non-numeric.
func segmentInt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return n
}
