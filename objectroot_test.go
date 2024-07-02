package ocfl_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
)

func TestParseObjectRootEntries(t *testing.T) {
	const objDecl = "0=ocfl_object_1.1"
	type testCase struct {
		input []fs.DirEntry
		want  ocfl.ObjectRootState
	}
	testCases := map[string]testCase{
		"nil input": {
			input: nil,
			want:  ocfl.ObjectRootState{},
		},
		"empty input": {
			input: []fs.DirEntry{},
			want:  ocfl.ObjectRootState{},
		},
		"regular namaste": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl},
			},
			want: ocfl.ObjectRootState{
				Spec:  ocfl.Spec1_1,
				Flags: ocfl.HasNamaste,
			},
		},
		"irregular namaste": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl, mode: fs.ModeIrregular},
			},
			want: ocfl.ObjectRootState{
				Spec:  ocfl.Spec1_1,
				Flags: ocfl.HasNamaste,
			},
		},
		"symlink namaste": {
			input: []fs.DirEntry{
				&dirEntry{name: objDecl, mode: fs.ModeSymlink},
			},
			want: ocfl.ObjectRootState{
				Invalid: []string{"0=ocfl_object_1.1"},
			},
		},
	}
	i := 0
	for name, kase := range testCases {
		t.Run(fmt.Sprintf("case %d %s", i, name), func(t *testing.T) {
			got := ocfl.ParseObjectRootDir(kase.input)
			be.DeepEqual(t, kase.want, *got)
		})
		i++
	}

	// cases from fixtures
	type testCaseFixture struct {
		name string
		want ocfl.ObjectRootState
	}
	fixtureDir := filepath.Join(`testdata`, `object-fixtures`, `1.0`)
	fixtureCases := []testCaseFixture{
		{
			name: filepath.Join(`bad-objects`, `E001_extra_dir_in_root`),
			want: ocfl.ObjectRootState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "sha512",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasSidecar | ocfl.HasNamaste,
				Invalid:     []string{"extra_dir"},
			},
		}, {
			name: filepath.Join(`bad-objects`, `E001_v2_file_in_root`),
			want: ocfl.ObjectRootState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "sha512",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasSidecar | ocfl.HasNamaste,
				Invalid:     []string{"v2"},
			},
		}, {
			name: filepath.Join(`warn-objects`, `W013_unregistered_extension`),
			want: ocfl.ObjectRootState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "sha512",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasSidecar | ocfl.HasNamaste | ocfl.HasExtensions,
			},
		}, {
			name: filepath.Join(`bad-objects`, `E058_no_sidecar`),
			want: ocfl.ObjectRootState{
				Spec:        ocfl.Spec1_0,
				SidecarAlg:  "",
				VersionDirs: []ocfl.VNum{ocfl.V(1)},
				Flags:       ocfl.HasInventory | ocfl.HasNamaste,
			},
		},
	}
	for _, fixCase := range fixtureCases {
		t.Run(fmt.Sprintf("fixture %s", fixCase.name), func(t *testing.T) {
			entries, err := os.ReadDir(filepath.Join(fixtureDir, fixCase.name))
			be.NilErr(t, err)
			got := ocfl.ParseObjectRootDir(entries)
			be.DeepEqual(t, fixCase.want, *got)
		})
	}
}

func TestGetObjectRoots(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.DirFS(filepath.Join(`testdata`, `object-fixtures`))
	t.Run("ok", func(t *testing.T) {
		const dir = "1.0/good-objects/spec-ex-full"
		obj, err := ocfl.GetObjectRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		be.Equal(t, fsys, obj.FS)
		be.Equal(t, dir, obj.Path)
		expect := ocfl.ObjectRootState{
			Spec:        ocfl.Spec1_0,
			SidecarAlg:  "sha512",
			VersionDirs: ocfl.VNums{ocfl.V(1), ocfl.V(2), ocfl.V(3)},
			Flags:       ocfl.HasNamaste | ocfl.HasInventory | ocfl.HasSidecar,
		}
		be.DeepEqual(t, expect, *obj.State)
	})
	t.Run("missing directory error", func(t *testing.T) {
		const dir = "not-existing"
		obj, err := ocfl.GetObjectRoot(ctx, fsys, dir)
		be.Zero(t, obj)
		be.True(t, err != nil)
		be.True(t, errors.Is(err, fs.ErrNotExist))
		var pathError *fs.PathError
		be.True(t, errors.As(err, &pathError))
		be.Equal(t, dir, pathError.Path)
	})
	t.Run("missing namaste error", func(t *testing.T) {
		const dir = "1.0"
		obj, err := ocfl.GetObjectRoot(ctx, fsys, dir)
		be.Zero(t, obj)
		be.True(t, err != nil)
		be.True(t, errors.Is(err, fs.ErrNotExist))
		be.True(t, errors.Is(err, ocfl.ErrObjectNamasteNotExist))
	})
}

func TestObjectRootValidateNamaste(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.DirFS(filepath.Join(`testdata`, `object-fixtures`))
	t.Run("ok", func(t *testing.T) {
		const dir = "1.0/good-objects/spec-ex-full"
		objroot := &ocfl.ObjectRoot{FS: fsys, Path: dir}
		be.NilErr(t, objroot.ValidateNamaste(ctx, ocfl.Spec1_0))
	})
	t.Run("missing namaste error", func(t *testing.T) {
		const dir = "1.0"
		objroot := &ocfl.ObjectRoot{FS: fsys, Path: dir}
		err := objroot.ValidateNamaste(ctx, ocfl.Spec1_0)
		be.True(t, err != nil)
		be.True(t, errors.Is(err, ocfl.ErrObjectNamasteNotExist))
	})
	t.Run("invalid namaste", func(t *testing.T) {
		const dir = "1.0/bad-objects/E007_bad_declaration_contents"
		objroot := &ocfl.ObjectRoot{FS: fsys, Path: dir}
		err := objroot.ValidateNamaste(ctx, ocfl.Spec1_0)
		be.True(t, err != nil)
		be.True(t, errors.Is(err, ocfl.ErrNamasteContents))
	})
}

func TestObjectRoot(t *testing.T) {
	ctx := context.Background()
	goodObjPath := "1.1/good-objects/spec-ex-full"
	goodObjSpec := ocfl.Spec("1.1")
	extensionObjPath := "1.1/warn-objects/W013_unregistered_extension"
	fsys := ocfl.DirFS(filepath.Join(`testdata`, `object-fixtures`))

	t.Run("OpenFile", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			_, err := objroot.OpenFile(ctx, "v3/inventory.json")
			be.NilErr(t, err)
		})
		t.Run("error on invalid path", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			_, err := objroot.OpenFile(ctx, "../file.txt")
			be.True(t, err != nil)
			be.True(t, errors.Is(err, fs.ErrInvalid))
		})
		t.Run("error on invalid object root path", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: "invalid/.."}
			_, err := objroot.OpenFile(ctx, "file.txt")
			be.True(t, err != nil)
			be.True(t, errors.Is(err, fs.ErrInvalid))
		})
	})

	t.Run("ReadDir", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			_, err := objroot.ReadDir(ctx, "v3")
			be.NilErr(t, err)
		})
		t.Run("error on invalid path", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			_, err := objroot.ReadDir(ctx, "../dir")
			be.True(t, err != nil)
			be.True(t, errors.Is(err, fs.ErrInvalid))
		})

		t.Run("error on invalid object root path", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: "invalid/.."}
			_, err := objroot.ReadDir(ctx, "dir")
			be.True(t, err != nil)
			be.True(t, errors.Is(err, fs.ErrInvalid))
		})
	})

	t.Run("ValidateNamaste", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			be.NilErr(t, objroot.ValidateNamaste(ctx, goodObjSpec))
		})
		t.Run("missing namaste error", func(t *testing.T) {
			// dir exists, but isn't an object
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: "1.0"}
			err := objroot.ValidateNamaste(ctx, goodObjSpec)
			be.True(t, err != nil)
			be.True(t, errors.Is(err, ocfl.ErrObjectNamasteNotExist))
		})
		t.Run("invalid namaste", func(t *testing.T) {
			dir := "1.0/bad-objects/E007_bad_declaration_contents"
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: dir}
			err := objroot.ValidateNamaste(ctx, ocfl.Spec1_0)
			be.True(t, err != nil)
			be.True(t, errors.Is(err, ocfl.ErrNamasteContents))
		})
	})

	t.Run("UnmarshalInventory", func(t *testing.T) {
		t.Run("root inventory", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			inv := struct {
				ID string `json:"id"`
			}{}
			be.NilErr(t, objroot.UnmarshalInventory(ctx, ".", &inv))
			be.Nonzero(t, inv.ID)
		})
		t.Run("version inventory", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			inv := struct {
				ID string `json:"id"`
			}{}
			be.NilErr(t, objroot.UnmarshalInventory(ctx, "v3", &inv))
			be.Nonzero(t, inv.ID)
		})
		t.Run("missing inventory", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			inv := struct {
				ID string `json:"id"`
			}{}
			err := objroot.UnmarshalInventory(ctx, "v2/content", &inv)
			be.True(t, err != nil)
			be.True(t, errors.Is(err, fs.ErrNotExist))
		})
		t.Run("invalid path", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			inv := struct {
				ID string `json:"id"`
			}{}
			err := objroot.UnmarshalInventory(ctx, "../v2", &inv)
			be.True(t, err != nil)
			be.True(t, errors.Is(err, fs.ErrInvalid))
		})
	})

	t.Run("ExtensionNames", func(t *testing.T) {
		t.Run("has extensions", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: extensionObjPath}
			exts, err := objroot.ExtensionNames(ctx)
			be.NilErr(t, err)
			slices.Sort(exts)
			be.DeepEqual(t, []string{"unregistered"}, exts)
		})
		t.Run("no extensions", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: goodObjPath}
			exts, err := objroot.ExtensionNames(ctx)
			be.NilErr(t, err)
			be.True(t, len(exts) == 0)
		})
		t.Run("has extensions", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: extensionObjPath}
			exts, err := objroot.ExtensionNames(ctx)
			be.NilErr(t, err)
			slices.Sort(exts)
			be.DeepEqual(t, []string{"unregistered"}, exts)
		})
		t.Run("not an object", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: "1.0"}
			_, err := objroot.ExtensionNames(ctx)
			be.True(t, err != nil)
			be.True(t, errors.Is(err, ocfl.ErrObjectNamasteNotExist))
		})
		t.Run("root path doesn't exist", func(t *testing.T) {
			objroot := &ocfl.ObjectRoot{FS: fsys, Path: "none"}
			_, err := objroot.ExtensionNames(ctx)
			be.True(t, err != nil)
			be.True(t, errors.Is(err, fs.ErrNotExist))
		})
	})
}

// dirEntry is used for testing object root parsing
type dirEntry struct {
	name string
	mode fs.FileMode
}

func (d dirEntry) Name() string      { return d.name }
func (d dirEntry) IsDir() bool       { return d.mode.IsDir() }
func (d dirEntry) Type() fs.FileMode { return d.mode.Type() }
func (d dirEntry) Info() (fs.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestObjectRoots(t *testing.T) {
	ctx := context.Background()
	fixtureDir := filepath.Join(`testdata`, `store-fixtures`, `1.0`)
	fsys := ocfl.DirFS(fixtureDir)
	// storage root path -> number of objects expected
	testCases := map[string]int{
		"good-stores/simple-root":              3,
		"good-stores/reg-extension-dir-root":   1,
		"good-stores/unreg-extension-dir-root": 1,
	}
	for dir, numObj := range testCases {
		t.Run(dir, func(t *testing.T) {
			foundStores := 0
			ocfl.ObjectRoots(ctx, fsys, dir)(func(obj *ocfl.ObjectRoot, err error) bool {
				be.NilErr(t, err)
				foundStores++
				return true
			})
			be.Equal(t, numObj, foundStores)
		})
	}

	t.Run("use ocfl.ObjecRootsFS", func(t *testing.T) {
		const dir = "dir"
		fsys := &objectRooter{}
		ocfl.ObjectRoots(ctx, fsys, dir)
		be.Equal(t, dir, fsys.calledWith)
	})
}

// objectRooter is a mock implementation of ocfl.ObjectRootFS
type objectRooter struct {
	calledWith string
}

func (r *objectRooter) OpenFile(_ context.Context, _ string) (fs.File, error) {
	return nil, errors.New("not implemented")
}

func (r *objectRooter) ReadDir(_ context.Context, _ string) ([]fs.DirEntry, error) {
	return nil, errors.New("not implemented")
}

func (r *objectRooter) ObjectRoots(ctx context.Context, dir string) ocfl.ObjectRootSeq {
	r.calledWith = dir
	return func(_ func(*ocfl.ObjectRoot, error) bool) {}
}
