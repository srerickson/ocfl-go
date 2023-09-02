package ocfl_test

import (
	"context"
	"strings"
	"testing"

	"github.com/srerickson/ocfl"
)

func TestStageAddFS(t *testing.T) {
	ctx := context.Background()
	data := map[string][]byte{
		"dir/README.txt": []byte("README content"),
		"dir/file.txt":   []byte("same file content"),
		"dir/file2.txt":  []byte("same file content"),
		"unstaged.txt":   []byte("this file is not staged"),
	}
	contentFS := newTestFS(data)
	t.Run("valid inputs", func(t *testing.T) {
		stage := ocfl.NewStage(ocfl.SHA256)
		if err := stage.AddFS(ctx, contentFS, "dir", ocfl.SHA1); err != nil {
			t.Fatal(err)
		}
		for name := range data {
			if !strings.HasPrefix(name, "dir/") {
				if stage.State.GetDigest(name) != "" {
					t.Fatalf("stage state shouldn't include digest for '%s'", name)
				}
				continue
			}
			name = strings.TrimPrefix(name, "dir/") // remove prefix
			dig := stage.State.GetDigest(name)
			if dig == "" {
				t.Fatalf("stage state does not included '%s' as expected", name)
			}
			if set := stage.GetFixity(dig); set[ocfl.SHA1] == "" {
				t.Fatal("expected stage to include SHA1 for all the new content")
			}
		}
	})
	t.Run("invalid inputs", func(t *testing.T) {
		stage := ocfl.NewStage(ocfl.SHA256)
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
	t.Run("valid inputs", func(t *testing.T) {
		stage := ocfl.NewStage(ocfl.SHA256)
		stage.SetFS(contentFS, "dir")
		if err := stage.AddPath(ctx, "README.txt", ocfl.SHA1); err != nil {
			t.Fatal(err)
		}
		for name := range data {
			if !strings.HasPrefix(name, "dir/") {
				if stage.State.GetDigest(name) != "" {
					t.Fatalf("stage state shouldn't include digest for '%s'", name)
				}
				continue
			}
			name = strings.TrimPrefix(name, "dir/") // remove prefix
			dig := stage.State.GetDigest(name)
			if dig == "" {
				t.Fatalf("stage state does not included '%s' as expected", name)
			}
			if set := stage.GetFixity(dig); set[ocfl.SHA1] == "" {
				t.Fatal("expected stage to include SHA1 for all the new content")
			}
		}
	})
	t.Run("invalid inputs", func(t *testing.T) {
		t.Run("without fs", func(t *testing.T) {
			stage := ocfl.NewStage(ocfl.SHA256)

			if err := stage.AddPath(ctx, "README.txt"); err == nil {
				t.Fatal("expected an error")
			}
		})
		t.Run("faulty FS", func(t *testing.T) {
			data := map[string][]byte{
				"dir/conflict":          []byte("'conflict' as a file"),
				"dir/conflict/file.txt": []byte("'conflict' as a parent directory"),
			}
			stage := ocfl.NewStage(ocfl.SHA256)
			stage.SetFS(newTestFS(data), "dir")
			if err := stage.AddPath(ctx, "conflict"); err != nil {
				t.Fatal()
			}
			if err := stage.AddPath(ctx, "conflict/file.txt"); err == nil {
				t.Fatal("expect an error")
			}
		})
		t.Run("missing path", func(t *testing.T) {
			stage := ocfl.NewStage(ocfl.SHA256)
			stage.SetFS(contentFS, "dir")
			if err := stage.AddPath(ctx, "missing"); err == nil {
				t.Fatal("expect an error")
			}
		})
	})
}

func TestStageUnsafeAddPathAs(t *testing.T) {
	t.Run("invalid inputs", func(t *testing.T) {
		t.Run("conflict in logical path", func(t *testing.T) {
			stage := ocfl.NewStage(ocfl.SHA256)
			digests := ocfl.DigestSet{ocfl.SHA256: `abc`}
			err := stage.UnsafeAddPathAs("", "dir/file", digests)
			if err != nil {
				t.Fatal(err)
			}
			err = stage.UnsafeAddPathAs("", "dir", digests)
			if err == nil {
				t.Error("expect an error when adding a path that already exists as a directory")
			}
			err = stage.UnsafeAddPathAs("", "dir/file/file", digests)
			if err == nil {
				t.Error("expect an error when adding a path that treats an existing file as a directory")
			}
		})
	})
}

func TestStageManifest(t *testing.T) {
	stage := ocfl.NewStage(ocfl.SHA256)

	// in manifest and state
	if err := stage.UnsafeAddPathAs("c-1", "l-1", ocfl.DigestSet{ocfl.SHA256: `abc1`}); err != nil {
		t.Fatal(err)
	}
	// in state but not manifest
	if err := stage.UnsafeAddPathAs("", "l-2", ocfl.DigestSet{ocfl.SHA256: `abc2`}); err != nil {
		t.Fatal(err)
	}
	// in manifest but not state
	if err := stage.UnsafeAddPathAs("c-3", "", ocfl.DigestSet{ocfl.SHA256: `abc3`}); err != nil {
		t.Fatal(err)
	}
	man, err := stage.Manifest()
	if err != nil {
		t.Fatal("Manifest() error:", err)
	}
	paths := man.PathMap()
	if l := len(paths); l != 1 {
		t.Errorf("Manifest() has more files than expected, got=%d, expect=%d", l, 1)
	}
	if d := paths["c-1"]; d == "" {
		t.Errorf("Manifest() didn't have expected value for path: got=%s, expected=%s", d, "abc1")
	}
}
