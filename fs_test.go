package ocfl_test

import (
	"bytes"
	"context"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/cloud"
	"github.com/srerickson/ocfl/backend/local"
	"github.com/srerickson/ocfl/backend/memfs"
	"gocloud.dev/blob/fileblob"
)

var (
	// goodObjects  = filepath.Join(testDataPath, `object-fixtures`, `1.0`, `good-objects`)
	warnObjects = filepath.Join(`testdata`, `object-fixtures`, `1.0`, `warn-objects`)
)

func newTestFS(data map[string][]byte) ocfl.FS {
	ctx := context.Background()
	fsys := memfs.New()
	for name, file := range data {
		_, err := fsys.Write(ctx, name, bytes.NewBuffer(file))
		if err != nil {
			panic(err)
		}
	}
	return fsys
}

func TestFiles(t *testing.T) {
	ctx := context.Background()
	testdata := filepath.Join(`testdata`)
	bucket, err := fileblob.OpenBucket(testdata, nil)
	if err != nil {
		t.Fatal(err)
	}
	local, err := local.NewFS(testdata)
	if err != nil {
		t.Fatal(err)
	}
	fss := map[string]ocfl.FS{
		"local":    local,
		"dirfs":    ocfl.DirFS(testdata),
		"fileblob": cloud.NewFS(bucket),
	}
	got := map[string][]string{}
	for fsysName, fsys := range fss {
		t.Run(fsysName, func(t *testing.T) {
			dir := ocfl.Dir(`object-fixtures`)
			dir.SkipDirFn = func(n string) bool {
				return len(strings.Split(n, "/")) > 4
			}
			ocfl.Files(ctx, fsys, dir, func(name string) error {
				if dir.SkipDir(path.Dir(name)) {
					t.Error("Files didn't respect SkipDirFn", name)
				}
				f, err := fsys.OpenFile(ctx, name)
				if err != nil {
					t.Error("File name isn't accessible")
				}
				if err == nil {
					defer f.Close()
				}
				got[fsysName] = append(got[fsysName], name)
				return nil
			})
			sort.Strings(got[fsysName])
		})
	}
	cmpTo := ""
	for fsysName, files := range got {
		if cmpTo == "" {
			cmpTo = fsysName
			continue
		}
		expect := got[cmpTo]
		if !reflect.DeepEqual(files, expect) {
			t.Errorf("%s and %s didn't get the same results from File: got=%v and expect=%v", fsysName, cmpTo, files, expect)
		}
	}
}
