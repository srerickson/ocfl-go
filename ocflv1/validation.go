package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
)

// validateVersion fully validates an ocfl v1.x version directory for ver in the object.
// the previous inventory may be nil.
func validateVersion(ctx context.Context, obj ocfl.ReadObject, dirNum ocfl.VNum, prev ocfl.Inventory, vldr *ocfl.Validation) {
	fsys := obj.FS()
	vDir := path.Join(obj.Path(), dirNum.String())
	// isHead := obj.Inventory().Head() == dirNum
	verContentDir := contentDir
	// what spec does the version use? Priority:
	//  1. spec in inventory (if present)
	//  2. prev spec (if present)
	//  3. 1.0
	verSpec := ocfl.Spec1_0
	if prev != nil {
		verSpec = prev.Spec()
	}
	// read the contents of the version directory
	entries, err := fsys.ReadDir(ctx, vDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		vldr.AddFatal(err)
		return
	}
	if len(entries) < 1 {
		// the version directory doesn't exist or it's empty
		err := fmt.Errorf("missing %s inventory file: %s", dirNum.String())
		vldr.AddWarn(ec(err, codes.W010(verSpec)))
		return
	}
	info := parseVersionDirState(entries)
	for _, f := range info.extraFiles {
		err := fmt.Errorf(`unexpected file in %s: %s`, dirNum, f)
		vldr.AddFatal(ec(err, codes.E015(verSpec)))
	}
	if info.hasInventory {
		invName := path.Join(vDir, inventoryFile)
		inv, invValid := ValidateInventory(ctx, fsys, invName, verSpec)
		if inv.ContentDirectory != "" {
			verContentDir = inv.ContentDirectory
		}

		// if isHead, check that digests match with root inventory

		// would be nice to add prefix to these errors
		vldr.AddErrors(invValid)
		vldr.AddInventory(&inventory{raw: *inv})
		verSpec = inv.Type.Spec
		if prev != nil {
			// the version content directory must be the same
			// the ocfl spec must >=
			// check that all version states in prev match the corresponding
			// version state in this inventory
			for _, v := range prev.Head().AsHead() {
				versionThis := inv.Versions[v]
				versionPrev := prev.Version(v.Num())
				vLogicalStateThis := logicalState{
					state:    versionThis.State,
					manifest: inv.Manifest,
				}
				vLogicalStatePrev := logicalState{
					state:    versionPrev.State(),
					manifest: prev.Manifest(),
				}
				if !vLogicalStateThis.Eq(vLogicalStatePrev) {
					err := fmt.Errorf("%s/inventory.json has different logical state in its %s version block than the previous inventory.json", dirNum, v)
					vldr.AddFatal(ec(err, codes.E066(verSpec)))
				}
				if versionThis.Message != versionPrev.Message() {
					err := fmt.Errorf("%s/inventory.json has different 'message' in its %s version block than the previous inventory.json", dirNum, v)
					vldr.AddWarn(ec(err, codes.W011(verSpec)))
				}
				if !reflect.DeepEqual(versionThis.User, versionPrev.User()) {
					err := fmt.Errorf("%s/inventory.json has different 'user' in its %s version block than the previous inventory.json", dirNum, v)
					vldr.AddWarn(ec(err, codes.W011(verSpec)))
				}
				if versionThis.Created != versionPrev.Created() {
					err := fmt.Errorf("%s/inventory.json has different 'created' in its %s version block than the previous inventory.json", dirNum, v)
					vldr.AddWarn(ec(err, codes.W011(verSpec)))
				}
			}
		}
	}
	for _, d := range info.dirs {
		// directory SHOULD only be content directory
		if d != verContentDir {
			err := fmt.Errorf(`extra directory in %s: %s`, dirNum, d)
			vldr.AddWarn(ec(err, codes.W002(verSpec)))
			continue
		}
		// add version content directory to validation state
		var added int
		var iterErr error
		ocfl.Files(ctx, fsys, path.Join(vDir, verContentDir))(func(info ocfl.FileInfo, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			// convert fs-relative path to object-relative path
			vldr.AddContentExists(strings.TrimPrefix(info.Path, obj.Path()+"/"))
			added++
			return true
		})
		if iterErr != nil {
			vldr.AddFatal(err)
			return
		}
		if added == 0 {
			// content directory exists but it's empty
			err := fmt.Errorf("content directory (%s) contains no files", verContentDir)
			vldr.AddFatal(ec(err, codes.E016(verSpec)))
		}
	}
	return
}

type versionDirState struct {
	hasInventory bool
	sidecarAlg   string
	extraFiles   []string
	dirs         []string
}

func parseVersionDirState(entries []fs.DirEntry) versionDirState {
	var info versionDirState
	for _, e := range entries {
		if e.Type().IsDir() {
			info.dirs = append(info.dirs, e.Name())
			continue
		}
		if e.Type().IsRegular() || e.Type() == fs.ModeIrregular {
			if e.Name() == inventoryFile {
				info.hasInventory = true
				continue
			}
			if strings.HasPrefix(e.Name(), inventoryFile+".") && info.sidecarAlg == "" {
				info.sidecarAlg = strings.TrimPrefix(e.Name(), inventoryFile+".")
				continue
			}
		}
		// unexpected files
		info.extraFiles = append(info.extraFiles, e.Name())
	}
	return info
}
