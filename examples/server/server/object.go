package server

import (
	"context"
	"errors"
	"iter"
	"slices"
	"time"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

func NewObject(ctx context.Context, base *ocfl.Object) (*Object, error) {
	inv := base.Inventory()
	if inv == nil {
		return nil, errors.New("object doesn't exist")
	}
	vers := inv.Head().Lineage()
	if len(vers) < 1 {
		return nil, errors.New("object has no versions")
	}
	obj := &Object{
		ID:              inv.ID(),
		DigestAlgorithm: inv.DigestAlgorithm().ID(),
		Versions:        make([]ObjectVersion, len(vers)-1),
	}
	slices.Reverse(vers)
	for i, vnum := range vers {
		verFS, err := base.OpenVersion(ctx, vnum.Num())
		if err != nil {
			return nil, err
		}
		objVersion := ObjectVersion{
			Num:     vnum.String(),
			Message: verFS.Message(),
			Created: verFS.Created(),
			User:    verFS.User(),
			Files:   VersionFiles(ctx, base, vnum.Num()),
		}
		if i == 0 {
			obj.Head = objVersion
			continue
		}
		obj.Versions[i-1] = objVersion
	}

	return obj, nil
}

type Object struct {
	ID              string
	DigestAlgorithm string
	Head            ObjectVersion
	Versions        []ObjectVersion
}

type ObjectVersion struct {
	Num     string
	Message string
	User    *ocfl.User
	Created time.Time
	Files   iter.Seq2[string, *digest.FileRef]
}

// // iterate over versions in order or preesntation (reversed)
// func (o Object) Versions() iter.Seq2[string, ocfl.ObjectVersion] {
// 	return func(yield func(string, ocfl.ObjectVersion) bool) {
// 		vers := o.Head().Lineage()
// 		slices.Reverse(vers)
// 		for _, v := range vers {
// 			if !yield(v.String(), o.Version(v.Num())) {
// 				return
// 			}
// 		}
// 	}
// }

// VersionFiles returns an iterator that yields
func VersionFiles(ctx context.Context, obj *ocfl.Object, num int) iter.Seq2[string, *digest.FileRef] {
	inv := obj.Inventory()
	if inv == nil {
		return func(yield func(string, *digest.FileRef) bool) {}
	}
	version := inv.Version(num)
	manifest := inv.Manifest()
	if version == nil || manifest == nil {
		return func(yield func(string, *digest.FileRef) bool) {}
	}
	return func(yield func(string, *digest.FileRef) bool) {
		statePathMap := version.State().PathMap()
		for logicalPath, dig := range statePathMap.SortedPaths() {
			contentPaths := manifest[dig]
			if len(contentPaths) < 1 {
				return
			}
			fileref := &digest.FileRef{
				FileRef: ocflfs.FileRef{
					FS:      obj.FS(),
					BaseDir: obj.Path(),
					Path:    contentPaths[0],
				},
				Algorithm: inv.DigestAlgorithm(),
				Digests:   inv.GetFixity(dig),
			}
			fileref.Digests[inv.DigestAlgorithm().ID()] = dig
			if err := fileref.Stat(ctx); err != nil {
				// log error but continue
				// return
			}
			if !yield(logicalPath, fileref) {
				return
			}
		}
	}
}

// this doesn't work so well because I need to stat the files.
// I can't stat concurrently without messing up the order, so
// may as well just have a map or slice instead of an  iterator:
// doesn't really get us anything to use iterator.
