package ocfl

import (
	"io/fs"
	"slices"
	"strings"
)

const (
	// HasNamaste indicates that an object root directory includes a NAMASTE
	// object declaration file
	HasNamaste objectStateFlag = 1 << iota
	// HasInventory indicates that an object root includes an "inventory.json"
	// file
	HasInventory
	// HasSidecar indicates that an object root includes an "inventory.json.*"
	// file (the inventory sidecar).
	HasSidecar
	// HasExtensions indicates that an object root includes a directory named
	// "extensions"
	HasExtensions
	//HasLogs indicates that an object root includes a directory named "logs"
	HasLogs

	maxObjectStateInvalid = 8
	objectDeclPrefix      = "0=" + NamasteTypeObject
	sidecarPrefix         = inventoryBase + "."
)

// ObjectState provides details of an OCFL object root based on the names of
// files and directories in the object's root. ParseObjectDir is typically
// used to create a new ObjectState from a slice of fs.DirEntry values.
type ObjectState struct {
	Spec        Spec            // the OCFL spec from the object's NAMASTE declaration file
	VersionDirs VNums           // version directories found in the object directory
	SidecarAlg  string          // digest algorithm used by the inventory sidecar file
	Invalid     []string        // non-conforming directory entries in the object root (max of 8)
	Flags       objectStateFlag // boolean attributes of the object root
}

type objectStateFlag uint8

// ParseObjectDir returns a new ObjectState based on contents of an
// object root directory.
func ParseObjectDir(entries []fs.DirEntry) *ObjectState {
	state := &ObjectState{}
	addInvalid := func(name string) {
		if len(state.Invalid) < maxObjectStateInvalid {
			state.Invalid = append(state.Invalid, name)
		}
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case e.IsDir():
			var v VNum
			switch {
			case name == logsDir:
				state.Flags |= HasLogs
			case name == extensionsDir:
				state.Flags |= HasExtensions
			case ParseVNum(name, &v) == nil:
				state.VersionDirs = append(state.VersionDirs, v)
			default:
				// invalid directory
				addInvalid(name)
			}
		case validFileType(e.Type()):
			switch {
			case name == inventoryBase:
				state.Flags |= HasInventory
			case strings.HasPrefix(name, sidecarPrefix):
				if state.HasSidecar() {
					// duplicate sidecar-like file
					addInvalid(name)
					break
				}
				state.SidecarAlg = strings.TrimPrefix(name, sidecarPrefix)
				state.Flags |= HasSidecar
			case strings.HasPrefix(name, objectDeclPrefix):
				if state.HasNamaste() {
					// duplicate namaste
					addInvalid(name)
					break
				}
				decl, err := ParseNamaste(name)
				if err != nil {
					addInvalid(name)
					break
				}
				state.Spec = decl.Version
				state.Flags |= HasNamaste
			default:
				// invalid file
				addInvalid(name)
			}
		default:
			// invalid mode type
			addInvalid(name)
		}
	}
	return state
}

// HasNamaste returns true if state's HasNamaste flag is set
func (state ObjectState) HasNamaste() bool {
	return state.Flags&HasNamaste > 0
}

// HasInventory returns true if state's HasInventory flag is set
func (state ObjectState) HasInventory() bool {
	return state.Flags&HasInventory > 0
}

// HasSidecar returns true if state's HasSidecar flag is set
func (state ObjectState) HasSidecar() bool {
	return state.Flags&HasSidecar > 0
}

// HasExtensions returns true if state's HasExtensions flag is set
func (state ObjectState) HasExtensions() bool {
	return state.Flags&HasExtensions > 0
}

// HasLogs returns true if state's HasLogs flag is set
func (state ObjectState) HasLogs() bool {
	return state.Flags&HasLogs > 0
}

// HasVersionDir returns true if the state's VersionDirs includes v
func (state ObjectState) HasVersionDir(v VNum) bool {
	return slices.Contains(state.VersionDirs, v)
}

// Empty returns true if the object root directory is empty
func (state ObjectState) Empty() bool {
	return state.Flags == 0 && len(state.VersionDirs) == 0 && len(state.Invalid) == 0
}

// Namaste returns state's Namaste value, which may be a zero value.
func (state ObjectState) Namaste() Namaste {
	var n Namaste
	if state.HasNamaste() {
		n.Version = state.Spec
		n.Type = NamasteTypeObject
	}
	return n
}
