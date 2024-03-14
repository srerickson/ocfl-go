package ocfl_test

import (
	"context"
	"strings"
	"testing"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/cloud"
	"github.com/srerickson/ocfl-go/backend/local"
	"gocloud.dev/blob/fileblob"
)

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
			fn := func(obj *ocfl.ObjectRoot) error {
				numobjs++
				if obj.SidecarAlg == "" {
					t.Error("algorithm not set for", obj.Path)
				}
				if !obj.HasInventory() {
					t.Error("HasInventory false for", obj.Path)
				}
				if !obj.HasSidecar() {
					t.Error("HasSidecar false for", obj.Path)
				}
				if err := obj.VersionDirs.Valid(); err != nil {
					t.Error("version dirs not valid for", obj.Path)
				}
				v3Fixture := "w001_zero_padded_versions"
				if strings.HasSuffix(obj.Path, v3Fixture) {
					if len(obj.VersionDirs) != 3 {
						t.Error(obj.Path, "should have 3 versions")
					}
				}
				extFixture := "W013_unregistered_extension"
				if strings.HasSuffix(obj.Path, extFixture) {
					if obj.Flags&ocfl.HasExtensions == 0 {
						t.Errorf(obj.Path, "should have extensions flag")
					}
				}
				return nil
			}
			err = ocfl.ObjectRoots(context.Background(), fsys, ocfl.PathSelector{}, fn)
			if err != nil {
				t.Fatal(err)
			}
			if numobjs != 12 {
				t.Fatalf("expected 12 objects to be called, got %d", numobjs)
			}
		})
	}

}
