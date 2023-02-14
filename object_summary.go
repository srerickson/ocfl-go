package ocfl

import (
	"context"
	"io/fs"
	"strings"
)

const (
	inventoryFile = "inventory.json"
	extensionsDir = "extensions"
)

// ObjectSummary provides an overview of an OCFL object based on file and
// directory names in the object's root directory
type ObjectSummary struct {
	Declaration      Declaration
	VersionDirs      VNums
	Algorithm        string
	HasInventoryFile bool
	HasExtensionsDir bool
	Unknown          []string
}

func NewObjectSummary(entries []fs.DirEntry) *ObjectSummary {
	var info ObjectSummary
	info.Declaration, _ = FindDeclaration(entries)
	for _, e := range entries {
		if e.IsDir() {
			if e.Name() == extensionsDir {
				info.HasExtensionsDir = true
				continue
			}
			v := VNum{}
			if err := ParseVNum(e.Name(), &v); err == nil {
				info.VersionDirs = append(info.VersionDirs, v)
				continue
			}
		} else {
			if e.Name() == inventoryFile {
				info.HasInventoryFile = true
				continue
			}
			if e.Name() == info.Declaration.Name() {
				continue
			}
			if info.Algorithm == "" && strings.HasPrefix(e.Name(), inventoryFile+".") {
				info.Algorithm = strings.TrimPrefix(e.Name(), inventoryFile+".")
				continue
			}
		}
		info.Unknown = append(info.Unknown, e.Name())
	}
	return &info
}

// ReadObjectSummary reads the directory p in fsys and retuns an ObjectSummary
func ReadObjectSummary(ctx context.Context, fsys FS, p string) (*ObjectSummary, error) {
	dir, err := fsys.ReadDir(ctx, p)
	if err != nil {
		return nil, err
	}
	return NewObjectSummary(dir), nil
}
