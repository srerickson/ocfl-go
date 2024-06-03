package ocfl_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
			got := ocfl.ParseObjectRootEntries(kase.input)
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
			got := ocfl.ParseObjectRootEntries(entries)
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
		be.NilErr(t, objroot.ValidateNamaste(ctx))
	})
	t.Run("missing namaste error", func(t *testing.T) {
		const dir = "1.0"
		objroot := &ocfl.ObjectRoot{FS: fsys, Path: dir}
		err := objroot.ValidateNamaste(ctx)
		be.True(t, err != nil)
		be.True(t, errors.Is(err, ocfl.ErrObjectNamasteNotExist))
	})
	t.Run("invalid namaste", func(t *testing.T) {
		const dir = "1.0/bad-objects/E007_bad_declaration_contents"
		objroot := &ocfl.ObjectRoot{FS: fsys, Path: dir}
		err := objroot.ValidateNamaste(ctx)
		be.True(t, err != nil)
		be.True(t, errors.Is(err, ocfl.ErrNamasteContents))
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
