package ocfl_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/memfs"
	"github.com/srerickson/ocfl/digest"
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

func TestStageAddFS(t *testing.T) {
	ctx := context.Background()
	data := map[string][]byte{
		"dir/README.txt": []byte("README content"),
		"dir/file.txt":   []byte("same file content"),
		"dir/file2.txt":  []byte("same file content"),
		"unstaged.txt":   []byte("this file is not staged"),
	}
	contentFS := newTestFS(data)
	t.Run("valid", func(t *testing.T) {
		stage, err := ocfl.NewStage(digest.SHA256(), digest.Map{})
		if err != nil {
			t.Fatal(err)
		}
		if err := stage.AddFS(ctx, contentFS, "dir", digest.SHA1()); err != nil {
			t.Fatal(err)
		}
		state := stage.State()
		for name := range data {
			if !strings.HasPrefix(name, "dir/") {
				if state.GetDigest(name) != "" {
					t.Fatalf("stage state shouldn't include digest for '%s'", name)
				}
				continue
			}
			name = strings.TrimPrefix(name, "dir/") // remove prefix
			dig := state.GetDigest(name)
			if dig == "" {
				t.Fatalf("stage state does not included '%s' as expected", name)
			}
			if set := stage.GetFixity(dig); set[digest.SHA1id] == "" {
				t.Fatal("expected stage to include SHA1 for all the new content")
			}
		}
	})
	t.Run("invalid", func(t *testing.T) {
		stage, err := ocfl.NewStage(digest.SHA256(), digest.Map{})
		if err != nil {
			t.Fatal(err)
		}
		t.Run("invalid root", func(t *testing.T) {
			err := stage.AddFS(ctx, contentFS, "../dir")
			if err == nil {
				t.Fatal("expected an error")
			}
		})
		t.Run("missing root", func(t *testing.T) {
			err := stage.AddFS(ctx, contentFS, "dir2")
			if err == nil {
				t.Fatal("expected an error")
			}
		})
		t.Run("faulty FS", func(t *testing.T) {
			data := map[string][]byte{
				"dir/conflict":          []byte("'conflict' as a file"),
				"dir/conflict/file.txt": []byte("'conflict' as a parent directory"),
			}
			invalidFS := newTestFS(data)
			err := stage.AddFS(ctx, invalidFS, "dir")
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	})
}

func TestStageAddPath(t *testing.T) {
	ctx := context.Background()
	data := map[string][]byte{
		"dir/README.txt": []byte("README content"),
		"unstaged.txt":   []byte("this file is not accessible in the stage"),
	}
	contentFS := newTestFS(data)
	t.Run("valid", func(t *testing.T) {
		stage, err := ocfl.NewStage(digest.SHA256(), digest.Map{})
		if err != nil {
			t.Fatal(err)
		}
		stage.SetFS(contentFS, "dir")
		if err := stage.AddPath(ctx, "README.txt", digest.SHA1()); err != nil {
			t.Fatal(err)
		}
		state := stage.State()
		for name := range data {
			if !strings.HasPrefix(name, "dir/") {
				if state.GetDigest(name) != "" {
					t.Fatalf("stage state shouldn't include digest for '%s'", name)
				}
				continue
			}
			name = strings.TrimPrefix(name, "dir/") // remove prefix
			dig := state.GetDigest(name)
			if dig == "" {
				t.Fatalf("stage state does not included '%s' as expected", name)
			}
			if set := stage.GetFixity(dig); set[digest.SHA1id] == "" {
				t.Fatal("expected stage to include SHA1 for all the new content")
			}
		}
	})
	t.Run("invalid", func(t *testing.T) {

		t.Run("without fs", func(t *testing.T) {
			stage, err := ocfl.NewStage(digest.SHA256(), digest.Map{})
			if err != nil {
				t.Fatal(err)
			}
			if err := stage.AddPath(ctx, "README.txt"); err == nil {
				t.Fatal("expected an error")
			}
		})
		t.Run("faulty FS", func(t *testing.T) {
			data := map[string][]byte{
				"dir/conflict":          []byte("'conflict' as a file"),
				"dir/conflict/file.txt": []byte("'conflict' as a parent directory"),
			}
			stage, err := ocfl.NewStage(digest.SHA256(), digest.Map{})
			if err != nil {
				t.Fatal(err)
			}
			stage.SetFS(newTestFS(data), "dir")
			if err := stage.AddPath(ctx, "conflict"); err != nil {
				t.Fatal()
			}
			if err := stage.AddPath(ctx, "conflict/file.txt"); err == nil {
				t.Fatal("expect an error")
			}
		})
		t.Run("missing path", func(t *testing.T) {
			stage, err := ocfl.NewStage(digest.SHA256(), digest.Map{})
			if err != nil {
				t.Fatal(err)
			}
			stage.SetFS(contentFS, "dir")
			if err := stage.AddPath(ctx, "missing"); err == nil {
				t.Fatal("expect an error")
			}
		})
	})
}
