package ocfl_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"path"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/ocfltest"
	"github.com/srerickson/ocfl/internal/pipeline"
	"github.com/srerickson/ocfl/internal/testfs"
)

func newTestStage(root fstest.MapFS, dir string, fixity ...digest.Alg) (*ocfl.Stage, error) {
	ctx := context.Background()
	fsys := testfs.NewMemFS()
	for name, file := range root {
		_, err := fsys.Write(ctx, name, bytes.NewBuffer(file.Data))
		if err != nil {
			return nil, err
		}
	}
	stage := ocfl.NewStage(digest.SHA256(), ocfl.StageRoot(fsys, dir))
	if len(root) > 0 {
		if err := stage.AddAllFromRoot(context.Background(), fixity...); err != nil {
			return nil, err
		}
	}
	return stage, nil
}

// validateStageDigests checks that all digest values added to the stage
// are correct.
func validateStageDigests(ctx context.Context, stage *ocfl.Stage) error {
	return stage.Walk(func(name string, n *ocfl.Index) error {
		if n.IsDir() {
			return nil
		}
		val, _, err := n.GetVal(".")
		if err != nil {
			return err
		}
		if len(val.Digests) == 0 {
			return fmt.Errorf("digest set for '%s' is missing", name)
		}
		if len(val.SrcPaths) == 0 {
			return nil
		}
		fsys, root := stage.Root()
		f, err := fsys.OpenFile(ctx, path.Join(root, val.SrcPaths[0]))
		if err != nil {
			return err
		}
		defer f.Close()
		if err := val.Digests.Validate(f); err != nil {
			return err
		}
		return nil
	})
}

func TestNewStage(t *testing.T) {
	stg, err := newTestStage(fstest.MapFS{
		"stage/dir/README.txt": &fstest.MapFile{Data: []byte("README content")},
		"stage/file.txt":       &fstest.MapFile{Data: []byte("file content")},
	}, "stage", digest.MD5())
	if err != nil {
		t.Fatal(err)
	}
	// check staged path
	_, isdir, err := stg.GetVal("dir")
	if err != nil {
		t.Fatal(err)
	}
	if !isdir {
		t.Fatal("expected a dir")
	}
	// check staged path
	info, isdir, err := stg.GetVal("dir/README.txt")
	if err != nil {
		t.Fatal(err)
	}
	if isdir {
		t.Fatal("expected a file")
	}
	algID := stg.DigestAlg().ID()
	if info.Digests[algID] == "" {
		t.Fatalf("expected a %s value", algID)
	}
	// manifest should include two entries
	man, err := stg.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if l := len(man.AllPaths()); l != 2 {
		t.Fatalf("expected 2 entries in the manifest, got %d: %v", l, man.AllPaths())
	}
	// version state should include three entries
	st := stg.VersionState()
	if l := len(st.AllPaths()); l != 2 {
		t.Fatalf("expected 2 entries in the state, got %v", st.AllPaths())
	}
	// validate stage digests
	if err := validateStageDigests(context.Background(), stg); err != nil {
		t.Fatal(fmt.Errorf("stage is invalid: %w", err))
	}
}

func TestStageCopy(t *testing.T) {
	fsys := fstest.MapFS{
		"stage/dir/README.txt": &fstest.MapFile{Data: []byte("README content")},
		"stage/file.txt":       &fstest.MapFile{Data: []byte("file content")},
		"stage/data.csv":       &fstest.MapFile{Data: []byte("1,2,3")},
	}
	t.Run("no errors", func(t *testing.T) {
		tests := [][]string{
			{"stage/dir", "stage/new"},
			{"stage/file.txt", "file.txt"},
			{"stage", "new-stage"},
		}
		for _, test := range tests {
			stg, err := newTestStage(fsys, ".")
			if err != nil {
				t.Fatal(err)
			}
			src := test[0]
			dst := test[1]
			if err := stg.Copy(src, dst); err != nil {
				t.Fatal(err)
			}
			srcV, srcIsD, err := stg.GetVal(src)
			if err != nil {
				t.Fatal(err)
			}
			dstV, dstIsD, err := stg.GetVal(dst)
			if err != nil {
				t.Fatal(dst)
			}
			if !reflect.DeepEqual(srcV, dstV) {
				t.Fatal("copy didn't copy stage values correctly")
			}
			if srcIsD != dstIsD {
				t.Fatal("copy doesn't have same file/dir type as src")
			}
			if err := validateStageDigests(context.Background(), stg); err != nil {
				t.Fatal(fmt.Errorf("stage is invalid: %w", err))
			}
		}
	})

	t.Run("err: copy path", func(t *testing.T) {
		stg, err := newTestStage(fsys, ".")
		if err != nil {
			t.Fatal(err)
		}
		tests := [][]string{
			{"stage", "stage/new"},
			{".", "stage2"},
			{"stage", "stage/dir/sub"},
		}
		for _, test := range tests {
			err := stg.Copy(test[0], test[1])
			if err == nil {
				t.Fatal("expected an error, got none")
			}
			if !errors.Is(err, ocfl.ErrCopyPath) {
				t.Fatal("expected ErrCopyPath")
			}
			if err := validateStageDigests(context.Background(), stg); err != nil {
				t.Fatal(fmt.Errorf("stage is invalid: %w", err))
			}
		}
	})

	t.Run("err: dst exists", func(t *testing.T) {
		stg, err := newTestStage(fsys, ".")
		if err != nil {
			t.Fatal(err)
		}
		src := "stage/data.csv"
		dst := "stage/file.txt"
		err = stg.Copy(src, dst)
		if err == nil {
			t.Fatal("expected an error, got none")
		}
		if !errors.Is(err, ocfl.ErrExists) {
			t.Fatal("expected ErrExists")
		}
		if err := validateStageDigests(context.Background(), stg); err != nil {
			t.Fatal(fmt.Errorf("stage is invalid: %w", err))
		}
	})

}

func TestStageWrite(t *testing.T) {
	ctx := context.Background()
	t.Run("no errors", func(t *testing.T) {
		rootFiles := fstest.MapFS{
			"stage/file.txt": &fstest.MapFile{Data: []byte("stuff")},
		}
		stg, err := newTestStage(rootFiles, "stage")
		if err != nil {
			t.Fatal(err)
		}
		// write a file to the stage
		content := []byte("file content")
		h := digest.SHA256().New()
		h.Write(content)
		sum := hex.EncodeToString(h.Sum(nil))
		size, err := stg.WriteFile(ctx, "tmp.txt", bytes.NewBuffer(content))
		if err != nil {
			t.Fatal(err)
		}
		if int(size) != len(content) {
			t.Fatalf("expected %d bytes to be written, not %d", len(content), size)
		}
		val, isdir, err := stg.GetVal("tmp.txt")
		if err != nil {
			t.Fatal(err)
		}
		if isdir {
			t.Fatal("expected a file, not a directory")
		}
		if got := val.Digests[digest.SHA256id]; got != sum {
			t.Fatalf("expected digest '%s', not '%s'", sum, got)
		}
		// read files from stage fsys
		fsys, dir := stg.Root()
		entries, err := fsys.ReadDir(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if l := len(entries); l != 2 {
			t.Fatalf("expected 2 file entries, not %d", l)
		}
		if n := entries[0].Name(); n != "file.txt" {
			t.Fatalf("expected first entry to be %s, not %s", "file.txt", n)
		}
		if n := entries[1].Name(); n != "tmp.txt" {
			t.Fatalf("expected second entry to be %s, not %s", "tmp.txt", n)
		}
		if err := validateStageDigests(context.Background(), stg); err != nil {
			t.Fatal(fmt.Errorf("stage is invalid: %w", err))
		}
	})

	t.Run("concurrent write", func(t *testing.T) {
		// an empty stg that is writable
		stg, err := newTestStage(nil, ".")
		if err != nil {
			t.Fatal(err)
		}
		// a seedFS for test content to write
		seed := rand.New(rand.NewSource(12881771))
		numFiles := 1000
		maxSize := 1000
		seedFS := ocfltest.GenerateFS(seed, numFiles, maxSize)
		// use a pipeline to concurrently write files from genFS to the stage
		setup := func(add func(string) error) error {
			return fs.WalkDir(seedFS, ".", func(n string, de fs.DirEntry, err error) error {
				if !de.Type().IsRegular() {
					return nil
				}
				return add(n) // add the file to the pipeline for xfer
			})
		}
		// transfer the file
		xfer := func(name string) (string, error) {
			f, err := seedFS.Open(name)
			if err != nil {
				return "", err
			}
			defer f.Close()
			if _, err := stg.WriteFile(ctx, name, f); err != nil {
				return "", err
			}
			return name, nil
		}
		// check that the file was written
		check := func(src string, dst string, err error) error {
			if err != nil {
				return err
			}
			fsys, root := stg.Root()
			f, err := fsys.OpenFile(ctx, path.Join(root, dst))
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := f.Stat(); err != nil {
				return err
			}
			return nil
		}
		// run the pipeline with 4 go routines
		if err := pipeline.Run(setup, xfer, check, 4); err != nil {
			t.Fatal(err)
		}
		// check that added files are in manifest
		man, err := stg.Manifest()
		if err != nil {
			t.Fatal(err)
		}
		if l := len(man.AllPaths()); l != numFiles {
			t.Fatalf("the stage manifest should include %d files, not %d", numFiles, l)
		}
		if err := validateStageDigests(context.Background(), stg); err != nil {
			t.Fatal(fmt.Errorf("stage is invalid: %w", err))
		}
	})

	t.Run("concurrent write, same path", func(t *testing.T) {
		// check if writing different content to the same path from different
		// go routines corrupts the stage
		ctx := context.Background()
		stg, err := newTestStage(nil, ".")
		if err != nil {
			t.Fatal(err)
		}
		times := 100
		for i := 0; i < times; i++ {
			name := fmt.Sprintf("file-%d.txt", i)
			wg := sync.WaitGroup{}
			numworkers := 5
			wg.Add(numworkers)
			for w := 0; w < numworkers; w++ {
				w := w
				go func() {
					defer wg.Done()
					reader := strings.NewReader(fmt.Sprintf("content for %d", w))
					stg.WriteFile(ctx, name, reader)
				}()
			}
			wg.Wait()
		}
		if l := len(stg.VersionState().AllPaths()); l != 100 {
			t.Fatalf("expected 100 entries in the version state, got %d", l)
		}
		if err := validateStageDigests(ctx, stg); err != nil {
			t.Fatal(fmt.Errorf("stage is invalid: %w", err))
		}
	})

}
