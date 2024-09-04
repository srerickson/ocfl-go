package diff

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/exp/maps"
)

type Result struct {
	Added    []string
	Removed  []string
	Modified []string
	Renamed  map[string]string
}

func Diff(aPaths, bPaths map[string]string) (result Result, err error) {
	addMap := map[string][]string{} // digest map of new files in b
	rmMap := map[string][]string{}  // digest map of missing files in b
	for aPath, aDigest := range aPaths {
		bDigest, inB := bPaths[aPath]
		switch {
		case !inB:
			// aPath is not in bPaths: it was removed
			rmMap[aDigest] = append(rmMap[aDigest], aPath)
		case inB && bDigest != aDigest:
			// modified
			result.Modified = append(result.Modified, aPath)
		}
	}
	for bPath, bDigest := range bPaths {
		if _, inA := aPaths[bPath]; !inA {
			// bPath is not in aPaths: it's new
			addMap[bDigest] = append(addMap[bDigest], bPath)
		}
	}
	// build renames by finding matchine digests in added / removed
	renamed := map[string]string{}
	for dig, addPaths := range addMap {
		rmPaths := rmMap[dig]
		sort.Strings(addPaths) // sort to make result deterministic
		sort.Strings(rmPaths)
		switch {
		case len(addPaths) > len(rmPaths):
			// create a rename pair for each rmPath
			for i, rmPath := range rmPaths {
				renamed[rmPath] = addPaths[i]
			}
			// remaining paths are addded
			result.Added = append(result.Added, addPaths[len(rmPaths):]...)
		default:
			// len(addPaths) <= len(rmPaths)
			// create a rename pair for each addPath
			for i, addPath := range addPaths {
				renamed[rmPaths[i]] = addPath
			}
			// remaining are removed
			result.Removed = append(result.Removed, rmPaths[len(addPaths):]...)
		}
	}
	for dig, rmPaths := range rmMap {
		if _, ok := addMap[dig]; ok {
			continue
		}
		result.Removed = append(result.Removed, rmPaths...)
	}
	if len(renamed) > 0 {
		result.Renamed = renamed
	}
	sort.Strings(result.Added)
	sort.Strings(result.Removed)
	sort.Strings(result.Modified)
	return
}

func (r Result) String() string {
	b := &strings.Builder{}
	for _, n := range r.Added {
		fmt.Fprintln(b, "+", n)
	}
	for _, n := range r.Removed {
		fmt.Fprintln(b, "-", n)
	}
	for _, n := range r.Modified {
		fmt.Fprintln(b, "~", n)
	}
	moved := maps.Keys(r.Renamed)
	sort.Strings(moved)
	for _, n := range moved {
		fmt.Fprintln(b, n, "->", r.Renamed[n])
	}
	return b.String()
}

func (diff Result) Empty() bool {
	return len(diff.Added) == 0 &&
		len(diff.Removed) == 0 &&
		len(diff.Modified) == 0 &&
		len(diff.Renamed) == 0
}