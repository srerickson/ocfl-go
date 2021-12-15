package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/srerickson/checksum"
	"github.com/srerickson/checksum/delta"
	"github.com/srerickson/ocfl/validation"
)

// ContentDiffErr represents an error due to
// unexpected content changes
type ContentDiffErr struct {
	Added       []string
	Removed     []string
	Modified    []string
	RenamedFrom []string
	RenamedTo   []string
}

func (e *ContentDiffErr) Error() string {
	return "unexpected files changes"
}

// ValidateObject validates the object at root
func ValidateObject(root fs.FS) *validation.Result {
	vr := &validation.Result{}
	obj, err := NewObjectReader(root)
	if err != nil {
		return vr.AddFatal(err, nil)
	}
	vr.Merge(obj.Validate())
	return vr
}

func (obj *ObjectReader) Validate() *validation.Result {
	result := &validation.Result{}
	var err error
	obj.inventory, err = obj.root.readInventory(`.`, true)
	if err != nil {
		return result.AddFatal(err, nil)
	}
	if err := obj.validateRoot(); err != nil {
		return result.AddFatal(err, nil)
	}
	for v := range obj.inventory.Versions {
		err := obj.validateVersionDir(v)
		if err != nil {
			return result.AddFatal(err, nil)
		}
	}
	if err := obj.validateContent(); err != nil {
		return result.AddFatal(err, nil)
	}
	if err := obj.validateFixity(); err != nil {
		return result.AddFatal(err, nil)
	}
	return nil
}

// validateRoot validates the object's root file structure. It checks
// existence of required files and absence of illegal files.
func (obj *ObjectReader) validateRoot() error {
	items, err := fs.ReadDir(obj.root, `.`)
	if err != nil {
		return err
	}
	match := dirMatch{
		ReqFiles: []string{
			inventoryFile,
			obj.inventory.SidecarFile(),
			objectDeclarationFile,
		},
		ReqDirs: obj.inventory.VersionDirs(),
		OptDirs: []string{"extensions"},
	}
	err = match.Match(items)
	if err != nil {
		if errors.Is(err, errDirMatchMissingFile) {
			if strings.Contains(err.Error(), objectDeclarationFile) {
				return validation.AsVErr(err, &validation.ErrE003)
			}
			if strings.Contains(err.Error(), obj.inventory.SidecarFile()) {
				return validation.AsVErr(err, &validation.ErrE058)
			}
			if strings.Contains(err.Error(), inventoryFile) {
				return validation.AsVErr(err, &validation.ErrE034)
			}
		}
		if errors.Is(err, errDirMatchInvalidFile) {
			return validation.AsVErr(err, &validation.ErrE001)
		}
		if errors.Is(err, errDirMatchMissingDir) {
			return validation.AsVErr(err, &validation.ErrE046)
		}
		if errors.Is(err, errDirMatchInvalidDir) {
			return validation.AsVErr(err, &validation.ErrE001)
		}
		return err
	}
	// err = versionSeqValid(obj.inventory.VersionDirs())
	// if err != nil {
	// 	return err
	// }
	err = obj.validateExtensionsDir()
	if err != nil {
		return err
	}
	return nil
}

func (obj *ObjectReader) validateVersionDir(v string) error {
	items, err := fs.ReadDir(obj.root, v)
	if err != nil {
		return err
	}
	match := dirMatch{
		FileRegexp: regexp.MustCompile(`^inventory\.json(\.[a-z0-9]+)?$`),
		DirRegexp:  regexp.MustCompile(`^.*$`),
	}
	err = match.Match(items)
	if err != nil {
		return validation.AsVErr(err, &validation.ErrE015)
	}
	var hasInventory bool
	for _, i := range items {
		if i.Type().IsRegular() && i.Name() == inventoryFile {
			hasInventory = true
		}
	}
	if hasInventory {
		inv, err := obj.root.readInventory(v, true)
		if err != nil {
			return err
		}
		if obj.inventory.Head == v {
			// if this is the HEAD version, root inventory should match this inventory
			if !bytes.Equal(obj.inventory.digest, inv.digest) {
				err := fmt.Errorf(`root inventory doesn't match inventory for %s`, v)
				return validation.AsVErr(err, &validation.ErrE064)
			}
		}
		return nil
	}
	// WARN no inventory
	return nil
}

func (obj *ObjectReader) validateContent() error {
	content, err := obj.Content()
	if err != nil {
		return err
	}
	// path -> digest
	allFiles, err := content.Paths()
	if err != nil {
		return err
	}
	// file and digests in content but not in manifest?
	manifest, err := obj.inventory.Manifest.Normalize()
	if err != nil {
		return err
	}
	paths, err := manifest.Paths()
	if err != nil {
		return err
	}
	changes := delta.New(paths, allFiles)

	if len(changes.Same()) != len(allFiles) || len(changes.Same()) != len(manifest) {
		err := &ContentDiffErr{
			Added:    changes.Added(),
			Removed:  changes.Removed(),
			Modified: changes.Modified(),
		}
		err.RenamedFrom, err.RenamedTo = changes.Renamed()
		if len(err.Modified) != 0 {
			return validation.AsVErr(err, &validation.ErrE092)
		}
		return validation.AsVErr(fmt.Errorf("content includes files not in manifest"), &validation.ErrE023)
	}
	// TODO E024 - empty directories
	return nil
}

func (obj *ObjectReader) validateExtensionsDir() error {
	items, err := fs.ReadDir(obj.root, "extensions")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	match := dirMatch{
		// only contain directories
		DirRegexp: regexp.MustCompile("^.*$"),
	}
	err = match.Match(items)
	if err != nil {
		return validation.AsVErr(err, &validation.ErrE067)
	}
	return nil
}

func (obj *ObjectReader) validateFixity() error {
	if obj.inventory == nil {
		return nil
	}
	if obj.inventory.Fixity == nil {
		return nil
	}
	for alg, digestMap := range obj.inventory.Fixity {
		digestMap, err := digestMap.Normalize()
		if err != nil {
			return validation.AsVErr(err, nil)
		}
		hash, err := newHash(alg)
		if err != nil {
			return validation.AsVErr(err, nil)
		}
		paths, err := digestMap.Paths()
		if err != nil {
			return validation.AsVErr(err, nil)
		}
		ctx, cancel := context.WithCancel(context.Background())
		pipe, err := checksum.NewPipe(obj.root,
			checksum.WithAlg(alg, hash),
			checksum.WithCtx(ctx),
		)
		if err != nil {
			cancel()
			return validation.AsVErr(err, nil)
		}
		go func() {
			defer pipe.Close()
			for path := range paths {
				pipe.Add(path)
			}
		}()
		for job := range pipe.Out() {
			if err := job.Err(); err != nil {
				cancel()
				return validation.AsVErr(err, nil)
			}
			sum, err := job.SumString(alg)
			if err != nil {
				cancel()
				return validation.AsVErr(err, nil)
			}
			if sum != paths[job.Path()] {
				cancel()
				err := fmt.Errorf("fixity check failed (%s): %s", alg, job.Path())
				return validation.AsVErr(err, &validation.ErrE093)
			}
		}
		cancel()
	}
	return nil
}
