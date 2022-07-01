package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/namaste"
	spec "github.com/srerickson/ocfl/spec"
)

const (
	NamastePrefix = "ocfl"
	layoutName    = "ocfl_layout.json"
	extensionsDir = "extensions"
)

var (
	ErrExtContents   = errors.New("storage root extension has invalid content")
	ErrEmptyDirs     = errors.New("storage root includes empty directories")
	ErrNonObject     = errors.New("storage root includes files that aren't part of an object")
	ErrObjectVersion = errors.New("storage root includes objects with higher OCFL version than the storage root")
	defaultLayout    = extensions.NewLayoutFlatDirect().(extensions.Layout)
)

func DefaultLayout() extensions.Layout {
	return defaultLayout
}

// ScanObjects walks fsys from root returning a map of object root paths.
func ScanObjects(ctx context.Context, fsys fs.FS, root string) (map[string]spec.Num, error) {
	paths := map[string]spec.Num{}
	queue := []string{root}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(queue) == 0 {
			break
		}
		dir := queue[0]
		queue = queue[1:]
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			return nil, err
		}
		if dir == path.Join(root, extensionsDir) {
			continue
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("%w: %s", ErrEmptyDirs, dir)
		}
		var numFiles int
		var nextDirs []string
		for _, e := range entries {
			if e.IsDir() {
				nextDirs = append(nextDirs, path.Join(dir, e.Name()))
			} else if e.Type().IsRegular() {
				numFiles++
			}
		}
		if numFiles == 0 || dir == root {
			queue = append(queue, nextDirs...)
			continue
		}
		decl, err := namaste.FindDelcaration(entries)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrNonObject, dir)
		}
		if decl.Type != "ocfl_object" {
			return nil, fmt.Errorf("%w: %s", ErrNonObject, dir)
		}
		paths[strings.TrimPrefix(dir, root+"/")] = decl.Version
	}
	return paths, nil
}
