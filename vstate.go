package ocfl

import "time"

// VState represents an OCFL object version by mapping logical path (from
// inventory's version state) to content paths (from inventory's manifest).
type VState struct {
	// map logical paths to content paths relative to the object root
	State   map[string][]string
	Message string
	Created time.Time
	User    struct {
		Name    string
		Address string
	}
}

// VDiff represents changes between two VStates
type VDiff struct {
	Add     []string // added logical paths
	Del     []string // removed logical paths
	Mod     []string // modified logical paths
	User    bool     // user changed
	Message bool     // message changed
	Created bool     // created changed
}

func (changes VDiff) Same() bool {
	if len(changes.Add) > 0 || len(changes.Del) > 0 || len(changes.Mod) > 0 ||
		changes.User || changes.Message || changes.Created {
		return false
	}
	return true
}

// Diff returns VDiff describing changes from stateA to stateB
func (stateA VState) Diff(stateB *VState) VDiff {
	hasCommon := func(a, b []string) bool {
		for _, i := range a {
			for _, j := range b {
				if i == j {
					return true
				}
			}
		}
		return false
	}
	var ch VDiff
	for logA, contA := range stateA.State {
		if contB, foundA := stateB.State[logA]; foundA {
			if !hasCommon(contA, contB) {
				// stateA logical path maps to different content in stateB
				ch.Mod = append(ch.Mod, logA)
			}
			continue
		}
		// stateA logical path not found in stateB
		ch.Del = append(ch.Del, logA)
	}
	for logB := range stateB.State {
		if _, foundB := stateA.State[logB]; !foundB {
			// stateB logical path not found in stateA
			ch.Add = append(ch.Add, logB)
		}
	}
	if stateA.Message != stateB.Message {
		ch.Message = true
	}
	if stateA.User != stateB.User {
		ch.User = true
	}
	if stateA.Created != stateB.Created {
		ch.Created = true
	}
	return ch
}
