package ocfl_test

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/ocflv1"
)

// full test of an object state
func testObjectState(ctx context.Context, state *ocfl.ObjectState, t *testing.T) {
	state.EachPath(func(pathName, dig string) error {
		t.Run("OpenFile(): "+pathName, func(t *testing.T) {
			t.Parallel()
			f, err := state.OpenFile(ctx, pathName)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			alg := state.Alg.ID()
			if err := (digest.Set{alg: dig}.Validate(f)); err != nil {
				t.Errorf("unexpected content: %s", err)
			}
			stat, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			gotName, expectName := stat.Name(), path.Base(pathName)
			if gotName != expectName {
				t.Errorf("fileinfo has wrong name: '%s', not '%s'", gotName, expectName)
			}
			if stat.IsDir() {
				t.Errorf("fileinfo isdir should be false")
			}
			gotTime, expectTime := stat.ModTime(), state.Created
			if !gotTime.Equal(expectTime) {
				t.Errorf("fileinfo has wrong modtime: %v, not %v", gotTime, expectTime)
			}
			gotMode, expectMode := stat.Mode(), ocfl.OBJECTSTATE_DEFAULT_FILEMODE
			if gotMode != expectMode {
				t.Errorf("fileinfo has wrong mode: %v, not %v", gotMode, expectMode)
			}
		})
		dirName := path.Dir(pathName)
		t.Run("ReadDir(): "+dirName, func(t *testing.T) {
			t.Parallel()
			entries, err := state.ReadDir(ctx, dirName)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) == 0 {
				t.Error("expect at least one entry")
			}
			for _, e := range entries {
				t.Run("DirEntry: "+e.Name(), func(t *testing.T) {
					info, err := e.Info()
					if err != nil {
						t.Fatalf("Info() for DirEntry '%s': %s", e.Name(), err)
					}
					if e.Name() != info.Name() {
						t.Errorf("FileInfo's Name='%s',DirEntry's Name='%s'", info.Name(), e.Name())
					}
				})
			}
		})
		return nil
	})
}

func TestObjecState(t *testing.T) {
	ctx := context.Background()

	t.Run("with zero values", func(t *testing.T) {
		var state ocfl.ObjectState
		t.Run("invalid path", func(t *testing.T) {
			_, err := state.OpenFile(ctx, "../file.txt")
			if !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("expected error to be fs.ErrInvalid, not %v", err)
			}
			_, err = state.ReadDir(ctx, "../dir")
			if !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("expected error to be fs.ErrInvalid, not %v", err)
			}
		})
		t.Run("invalid path", func(t *testing.T) {
			_, err := state.OpenFile(ctx, "file.txt")
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("expected error to be fs.ErrNotExist, not %v", err)
			}
			_, err = state.ReadDir(ctx, "dir")
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("expected error to be fs.ErrNotExist, not %v", err)
			}
		})
	})

	t.Run("fixures", func(t *testing.T) {
		fixtures := filepath.Join(`testdata`, `object-fixtures`, `1.1`)
		fsys := ocfl.DirFS(fixtures)
		runTestsFn := func(obj *ocflv1.Object) error {
			inv, err := obj.Inventory(ctx)
			if err != nil {
				return err
			}
			// test all version states
			for vnum := range inv.Versions {
				state, err := obj.ObjectState(ctx, vnum.Num())
				if err != nil {
					return err
				}
				name := obj.Path + `/` + vnum.String()
				t.Run(name, func(t *testing.T) {
					testObjectState(ctx, state, t)
				})
			}
			return nil
		}
		// add all version state of all good objects to states
		if err := ocflv1.ScanObjects(ctx, fsys, "good-objects", runTestsFn, nil); err != nil {
			t.Fatal(err)
		}
		// add all versions state of all warn objects to stattes
		if err := ocflv1.ScanObjects(ctx, fsys, "warn-objects", runTestsFn, nil); err != nil {
			t.Fatal(err)
		}
	})

}