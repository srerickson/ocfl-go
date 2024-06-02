package ocfl_test

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/cloud"
	"github.com/srerickson/ocfl-go/backend/local"
	"gocloud.dev/blob/fileblob"
)

var warnObjects = filepath.Join(`testdata`, `object-fixtures`, `1.0`, `warn-objects`)

func TestNewObjectRootState(t *testing.T) {
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
		"single regular namaste": {
			input: []fs.DirEntry{
				&dirEntry{name: "0=ocfl_object_1.1"},
			},
			want: ocfl.ObjectRootState{
				Spec:  ocfl.Spec1_1,
				Flags: ocfl.HasNamaste,
			},
		},
	}
	i := 0
	for name, kase := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, name), func(t *testing.T) {
			got := ocfl.NewObjectRootState(kase.input)
			be.DeepEqual(t, kase.want, *got)
		})
		i++
	}
}

func TestObjectRoots(t *testing.T) {
	b, err := fileblob.OpenBucket(warnObjects, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	localfs, err := local.NewFS(warnObjects)
	if err != nil {
		t.Fatal(err)
	}
	fss := map[string]ocfl.FS{
		"fileblob": cloud.NewFS(b),
		"dirfs":    ocfl.DirFS(warnObjects),
		"localfs":  localfs,
	}
	for fsName, fsys := range fss {
		t.Run(fsName, func(t *testing.T) {
			numobjs := 0
			fn := func(obj *ocfl.ObjectRoot, err error) bool {
				if err != nil {
					t.Error(err)
					return false
				}
				numobjs++
				if obj.State.SidecarAlg == "" {
					t.Error("algorithm not set for", obj.Path)
				}
				if !obj.State.HasInventory() {
					t.Error("HasInventory false for", obj.Path)
				}
				if !obj.State.HasSidecar() {
					t.Error("HasSidecar false for", obj.Path)
				}
				if err := obj.State.VersionDirs.Valid(); err != nil {
					t.Error("version dirs not valid for", obj.Path)
				}
				v3Fixture := "w001_zero_padded_versions"
				if strings.HasSuffix(obj.Path, v3Fixture) {
					if len(obj.State.VersionDirs) != 3 {
						t.Error(obj.Path, "should have 3 versions")
					}
				}
				extFixture := "W013_unregistered_extension"
				if strings.HasSuffix(obj.Path, extFixture) {
					if obj.State.Flags&ocfl.HasExtensions == 0 {
						t.Errorf(obj.Path, "should have extensions flag")
					}
				}
				return true
			}
			ocfl.ObjectRoots(context.Background(), fsys, ".")(fn)
			if numobjs != 12 {
				t.Fatalf("expected 12 objects to be called, got %d", numobjs)
			}
		})
	}
}

type dirEntry struct {
	name string
	typ  fs.FileMode
}

func (d dirEntry) Name() string      { return d.name }
func (d dirEntry) IsDir() bool       { return d.typ.IsDir() }
func (d dirEntry) Type() fs.FileMode { return d.typ.Type() }
func (d dirEntry) Info() (fs.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}
