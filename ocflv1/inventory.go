package ocflv1

import (
	"sort"
	"time"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/object"
	"github.com/srerickson/ocfl/spec"
)

// Inventory represents contents of an OCFL v1.x inventory.json file
type Inventory struct {
	ID               string                     `json:"id"`
	Type             spec.InventoryType         `json:"type"`
	DigestAlgorithm  digest.Alg                 `json:"digestAlgorithm"`
	Head             object.VNum                `json:"head"`
	ContentDirectory string                     `json:"contentDirectory,omitempty"`
	Manifest         *digest.Map                `json:"manifest"`
	Versions         map[object.VNum]*Version   `json:"versions"`
	Fixity           map[digest.Alg]*digest.Map `json:"fixity,omitempty"`

	digest string
}

// Version represents object version state and metadata
type Version struct {
	Created time.Time   `json:"created"`
	State   *digest.Map `json:"state"`
	Message string      `json:"message,omitempty"`
	User    *User       `json:"user,omitempty"`
}

// User represent a Version's user entry
type User struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address,omitempty"`
}

// VNums returns a sorted slice of VNums corresponding to the keys in the
// inventory's 'versions' block.
func (inv *Inventory) VNums() []object.VNum {
	vnums := make([]object.VNum, len(inv.Versions))
	i := 0
	for v := range inv.Versions {
		vnums[i] = v
		i++
	}
	sort.Sort(object.VNumSeq(vnums))
	return vnums
}

// VState returns a pointer to a VState representing the logical state of the
// version num. If num is an empty value, the head version is used. If the
// version does not exist for num, nil is returned.
func (inv *Inventory) VState(num object.VNum) *object.VState {
	if num == object.V0 {
		num = inv.Head
	}
	ver, exists := inv.Versions[num]
	if !exists {
		return nil
	}
	allPaths := ver.State.AllPaths()
	state := &object.VState{
		State:   make(map[string][]string, len(allPaths)),
		Created: ver.Created,
		Message: ver.Message,
	}
	if ver.User != nil {
		state.User.Address = ver.User.Address
		state.User.Name = ver.User.Name
	}
	for p, d := range allPaths {
		state.State[p] = inv.Manifest.DigestPaths(d)
	}
	return state
}
