package ocflv1

import (
	"errors"
	"fmt"
	"strings"

	"github.com/srerickson/ocfl"
)

const (
	inManifest = locFlag(1 << iota) // digest from manifest
	inFixity                        // digest from fixity
	inRootInv                       // digest from root inventory
	inVerInv                        // digest from version directory inventory
)

// pathLedger is used internally during validation to track content digests
// from multiple invenetories. Think of it as a union of all manifests in an
// object.
type pathLedger struct {
	paths map[string]*pathInfo
	// track all uniq inventories added
	inventories map[ocfl.VNum]locFlag
}

type pathInfo struct {
	existsIn ocfl.VNum
	digests  map[string]*digestInfo // alg -> digestInfo
}

func (pi *pathInfo) locations() map[ocfl.VNum]locFlag {
	var loc = map[ocfl.VNum]locFlag{}
	for _, dinfo := range pi.digests {
		for v, f := range dinfo.locs {
			loc[v] = loc[v] | f
		}
	}
	return loc
}

// referencedIn returns bool indicating the path is referenced
// in object with ver with flags flags
func (pi *pathInfo) referencedIn(ver ocfl.VNum, flag locFlag) bool {
	for _, dinfo := range pi.digests {
		for v, f := range dinfo.locs {
			if ver == v && flag&f > 0 {
				return true
			}
		}
	}
	return false
}

type digestInfo struct {
	digest string
	locs   map[ocfl.VNum]locFlag
}

func (l *pathLedger) addInventory(inv *Inventory, isRoot bool) error {
	alg := inv.DigestAlgorithm
	ver := inv.Head
	flag := inRootInv
	if !isRoot {
		flag = inVerInv
	}

	// add root inventory manifest to ledger
	for p, d := range inv.Manifest.AllPaths() {
		err := l.addPathDigest(p, alg, d, ver, flag|inManifest)
		if err != nil {
			return err
		}
	}
	// track all inventories added to the ledger
	if l.inventories == nil {
		l.inventories = make(map[ocfl.VNum]locFlag)
	}
	if flag.InRoot() {
		// check that root hasn't already been added with a different vnum
		for v, f := range l.inventories {
			if f.InRoot() && v != ver {
				err := errors.New("DEBUG: root inventory with different head than previously added root inventory")
				return err
			}
		}
	}
	l.inventories[ver] = flag | inManifest
	// add any paths in inventory's fixity
	if inv.Fixity != nil {
		for alg, dm := range inv.Fixity {
			for p, d := range dm.AllPaths() {
				err := l.addPathDigest(p, alg, d, ver, flag|inFixity)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ledg *pathLedger) addPathExists(p string, ver ocfl.VNum) {
	if ledg.paths == nil {
		ledg.paths = make(map[string]*pathInfo)
	}
	if _, exists := ledg.paths[p]; !exists {
		ledg.paths[p] = &pathInfo{
			existsIn: ver,
		}
	}
	ledg.paths[p].existsIn = ver
}

func (ledg *pathLedger) addPathDigest(path string, alg string, dig string, ver ocfl.VNum, flags locFlag) error {
	if ledg.paths == nil {
		ledg.paths = make(map[string]*pathInfo)
	}
	info, exists := ledg.paths[path]
	if !exists {
		// create path->alg->loc
		ledg.paths[path] = &pathInfo{
			digests: map[string]*digestInfo{
				alg: {
					digest: dig,
					locs:   map[ocfl.VNum]locFlag{ver: flags},
				},
			},
		}
		return nil
	}
	e, exists := info.digests[alg]
	if !exists {
		// add alg->loc to path entry
		ledg.paths[path].digests[alg] = &digestInfo{
			digest: dig,
			locs:   map[ocfl.VNum]locFlag{ver: flags},
		}
		return nil
	}
	if !strings.EqualFold(e.digest, dig) {
		return &ChangedDigestErr{
			Path: path,
			Alg:  alg,
		}
	}
	e.locs[ver] = e.locs[ver] | flags
	return nil
}

func (ledg *pathLedger) getDigest(path string, alg string) (*digestInfo, bool) {
	if pInfo, exists := ledg.paths[path]; exists {
		if dInfo, exists := pInfo.digests[alg]; exists {
			return dInfo, true
		}
	}
	return nil, false
}

type locFlag uint8

func (l locFlag) InVerInv() bool   { return l&inVerInv != 0 }
func (l locFlag) InRoot() bool     { return l&inRootInv != 0 }
func (l locFlag) InFixity() bool   { return l&inFixity != 0 }
func (l locFlag) InManifest() bool { return l&inManifest != 0 }

func (l locFlag) String() string {
	var ret string
	if l.InVerInv() {
		ret = "version inventory"
	} else {
		ret = "root inventory"
	}
	if l.InFixity() {
		ret += " (fixity)"
	} else {
		ret += " (manifest)"
	}
	return ret
}

// ChangedDigestErr: different digests for same content/alg in two locations
type ChangedDigestErr struct {
	Path string
	Alg  string
}

func (err ChangedDigestErr) Error() string {
	return fmt.Sprintf("divergent %s digests found for %s", err.Alg, err.Path)
}

// ContentDigestErr content digest doesn't match recorded digest
type ContentDigestErr struct {
	Path   string
	Alg    string
	Entry  digestInfo
	Digest string
}

func (err ContentDigestErr) Error() string {
	exp := err.Entry.digest
	got := err.Digest
	return fmt.Sprintf("the %s for %s has changed from '%s' to '%s'", err.Alg, err.Path, exp, got)
}
