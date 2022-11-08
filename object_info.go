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

// ObjInfo provides general information on an object
// based on file and directories in the object root.
type ObjInfo struct {
	Declaration      Declaration
	VersionDirs      VNumSeq
	Algorithm        string
	HasInventoryFile bool
	HasExtensionsDir bool
	Unknown          []string
}

func ObjInfoFromFS(dir []fs.DirEntry) *ObjInfo {
	var info ObjInfo
	info.Declaration, _ = FindDeclaration(dir)
	for _, e := range dir {
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

func ReadObjInfo(ctx context.Context, fsys FS, p string) (*ObjInfo, error) {
	dir, err := fsys.ReadDir(ctx, p)
	if err != nil {
		return nil, err
	}
	return ObjInfoFromFS(dir), nil
}
