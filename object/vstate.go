package object

import "time"

// VState represents the complete state of an object.
type VState struct {
	// State maps logical paths to a slice of content paths relative to an object/stage root
	State   map[string][]string
	Message string
	Created time.Time
	User    struct {
		Name    string
		Address string
	}
}

// changeSet compares to version states returns the changeSet.
type VersionChanges struct {
	Add     []string
	Del     []string
	Mod     []string
	User    bool // user changed
	Message bool // message changed
	Created bool // created changed
}

func (changes VersionChanges) Same() bool {
	if len(changes.Add) > 0 || len(changes.Del) > 0 || len(changes.Mod) > 0 ||
		changes.User || changes.Message || changes.Created {
		return false
	}
	return true
}

// Changes returns VersionChanges describing changes from stateA to stateB
func (stateA VState) Changes(stateB *VState) VersionChanges {
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
	var ch VersionChanges
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
