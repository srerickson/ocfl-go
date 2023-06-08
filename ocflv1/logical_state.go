package ocflv1

import (
	"sort"

	"github.com/srerickson/ocfl/digest"
)

// LogicalState represents a mapping of logical paths to a ManifestEntries
type LogicalState struct {
	Alg   digest.Alg
	Paths map[string]ManifestEntry
}

// ManifestEntry
type ManifestEntry struct {
	ContentPaths []string
	Digest       string
}

// SameContentAs returns true if both states include the same logical
// paths and the respective content paths. Otherwise, it returns false. It does
// not check the digest algorithm or digest values, making it appropriate in
// cases where the digest algorithm may have changed.
func (state LogicalState) SameContentAs(other LogicalState) bool {
	for l, entry := range state.Paths {
		otherEntry, exists := other.Paths[l]
		if !exists {
			return false
		}
		if len(entry.ContentPaths) != len(otherEntry.ContentPaths) {
			return false
		}
		sort.Strings(entry.ContentPaths)
		sort.Strings(otherEntry.ContentPaths)
		for i, p := range entry.ContentPaths {
			if otherEntry.ContentPaths[i] != p {
				return false
			}
		}
	}
	for l := range other.Paths {
		if _, ok := state.Paths[l]; !ok {
			return false
		}
	}
	return true
}
