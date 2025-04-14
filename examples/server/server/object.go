package server

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"slices"
	"time"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	// "github.com/srerickson/ocfl-go/fs"
)

func NewObject(ctx context.Context, base *ocfl.Object) (*Object, error) {
	inv := base.Inventory()
	if inv == nil {
		return nil, errors.New("object doesn't exist")
	}
	manifest := inv.Manifest()
	vers := inv.Head().Lineage()
	obj := &Object{
		ID:              inv.ID(),
		DigestAlgorithm: inv.DigestAlgorithm().ID(),
		Versions:        make([]ObjectVersion, len(vers)),
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
			Files:   make([]ObjectVersionFile, 0, verFS.State().NumPaths()),
		}
		pathMap := verFS.State().PathMap()

		err = fs.WalkDir(verFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			digest := pathMap[path]
			contentPath := ""
			if paths := manifest[digest]; len(paths) > 0 {
				contentPath = paths[0]
			}

			f := ObjectVersionFile{
				Name:         path,
				DownloadPath: downloadPath(inv.ID(), contentPath),
				Size:         info.Size(),
				//Digests digest.Set
			}
			objVersion.Files = append(objVersion.Files, f)
			return nil
		})
		if err != nil {
			return nil, err
		}
		obj.Versions[i] = objVersion
	}

	return obj, nil
}

type Object struct {
	ID              string
	DigestAlgorithm string
	Versions        []ObjectVersion
}

type ObjectVersion struct {
	Num     string
	Message string
	User    *ocfl.User
	Created time.Time
	Files   []ObjectVersionFile
}

type ObjectVersionFile struct {
	Name         string
	DownloadPath string
	Size         int64
	Digests      digest.Set
}

func downloadPath(id string, contentPath string) string {
	if contentPath == "" {
		return ""
	}
	return "/download/" + url.PathEscape(id) + "/" + url.PathEscape(contentPath)
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
