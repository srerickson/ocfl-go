package logical_test

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/logical-fs"
)

// LogicalFS implements io/fs.FS
var _ fs.FS = (*logical.LogicalFS)(nil)

func TestLogicalFS(t *testing.T) {
	ctx := context.Background()
	content := map[string]string{}
	for i := range 1000 {
		name := fmt.Sprintf("file-%d", i)
		content[name] = name + "-content"
	}
	baseFS := testOCFLFS(content)
	t.Run("fstest-single-subdir", func(t *testing.T) {
		refs := map[string]string{}
		for name := range content {
			refs["dir/"+name] = name
		}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)
		fstest.TestFS(logicalFS, slices.Collect(maps.Keys(refs))...)
	})

	t.Run("fstest-many-subdir", func(t *testing.T) {
		refs := map[string]string{}
		for name := range content {
			refs[name+"/"+name] = name
		}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)
		fstest.TestFS(logicalFS, slices.Collect(maps.Keys(refs))...)
	})
}

func TestLogicalFSEdgeCases(t *testing.T) {
	ctx := context.Background()
	content := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
	}
	baseFS := testOCFLFS(content)

	t.Run("open with invalid path", func(t *testing.T) {
		refs := map[string]string{"file1.txt": "file1.txt"}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		_, err := logicalFS.Open("../invalid")
		if err == nil {
			t.Fatal("expected error for invalid path")
		}
	})

	t.Run("read from directory returns error", func(t *testing.T) {
		refs := map[string]string{"dir/file1.txt": "file1.txt"}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		f, err := logicalFS.Open("dir")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		buf := make([]byte, 10)
		_, err = f.Read(buf)
		if err == nil {
			t.Fatal("expected error when reading from directory")
		}
	})

	t.Run("open non-existent file in underlying fs", func(t *testing.T) {
		// refs points to a file that doesn't exist in baseFS
		refs := map[string]string{"logical.txt": "nonexistent.txt"}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		_, err := logicalFS.Open("logical.txt")
		if err == nil {
			t.Fatal("expected error opening non-existent file")
		}
	})

	t.Run("dir entry info", func(t *testing.T) {
		refs := map[string]string{
			"dir/file1.txt": "file1.txt",
			"dir/file2.txt": "file2.txt",
		}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		dirFile, err := logicalFS.Open(".")
		if err != nil {
			t.Fatal(err)
		}
		defer dirFile.Close()

		readDirFile, ok := dirFile.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected ReadDirFile")
		}

		entries, err := readDirFile.ReadDir(-1)
		if err != nil {
			t.Fatal(err)
		}

		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}

		entry := entries[0]
		if entry.Name() != "dir" {
			t.Errorf("expected name 'dir', got %q", entry.Name())
		}
		if !entry.IsDir() {
			t.Error("expected entry to be a directory")
		}
		if !entry.Type().IsDir() {
			t.Error("expected entry type to be directory")
		}

		// Test Info() method
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Name() != "dir" {
			t.Errorf("expected info name 'dir', got %q",
				info.Name())
		}
		if !info.Mode().IsDir() {
			t.Error("expected info mode to be directory")
		}
		if !info.ModTime().Equal(created) {
			t.Errorf("modtime mismatch: got %v, want %v",
				info.ModTime(), created)
		}
		if info.Sys() != nil {
			t.Error("expected Sys() to return nil")
		}
	})

	t.Run("file stat and info", func(t *testing.T) {
		refs := map[string]string{"file1.txt": "file1.txt"}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		f, err := logicalFS.Open("file1.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}

		if stat.Name() != "file1.txt" {
			t.Errorf("expected name 'file1.txt', got %q",
				stat.Name())
		}
		if stat.IsDir() {
			t.Error("expected not to be a directory")
		}
		if stat.Size() != int64(len("content1")) {
			t.Errorf("expected size %d, got %d",
				len("content1"), stat.Size())
		}
		if !stat.ModTime().Equal(created) {
			t.Error("modtime mismatch")
		}
		if stat.Sys() != nil {
			t.Error("expected Sys() to return nil")
		}
	})

	t.Run("directory stat and modtime", func(t *testing.T) {
		refs := map[string]string{"dir/file1.txt": "file1.txt"}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		f, err := logicalFS.Open("dir")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}

		if !stat.IsDir() {
			t.Error("expected directory")
		}
		if !stat.ModTime().Equal(created) {
			t.Errorf("directory modtime mismatch: got %v, want %v",
				stat.ModTime(), created)
		}
	})

	t.Run("directory close on nil file", func(t *testing.T) {
		refs := map[string]string{"dir/file1.txt": "file1.txt"}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		f, err := logicalFS.Open("dir")
		if err != nil {
			t.Fatal(err)
		}

		// Close should not error even though underlying File is nil
		err = f.Close()
		if err != nil {
			t.Fatalf("unexpected error closing directory: %v", err)
		}
	})

	t.Run("dir entry info error when file doesn't exist",
		func(t *testing.T) {
			// Create a logical fs where a directory entry points to a
			// non-existent file
			refs := map[string]string{
				"dir/good.txt": "file1.txt",
				"dir/bad.txt":  "nonexistent.txt",
			}
			created := time.Now()
			logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

			dirFile, err := logicalFS.Open("dir")
			if err != nil {
				t.Fatal(err)
			}
			defer dirFile.Close()

			readDirFile, ok := dirFile.(fs.ReadDirFile)
			if !ok {
				t.Fatal("expected ReadDirFile")
			}

			entries, err := readDirFile.ReadDir(-1)
			if err != nil {
				t.Fatal(err)
			}

			// Find the bad entry
			var badEntry fs.DirEntry
			for _, e := range entries {
				if e.Name() == "bad.txt" {
					badEntry = e
					break
				}
			}
			if badEntry == nil {
				t.Fatal("could not find bad.txt entry")
			}

			// Calling Info() should return an error
			_, err = badEntry.Info()
			if err == nil {
				t.Fatal("expected error calling Info() on bad entry")
			}
		})

	t.Run("readdir paging", func(t *testing.T) {
		// Create a directory with multiple files to test paging
		refs := map[string]string{
			"dir/file1.txt": "file1.txt",
			"dir/file2.txt": "file2.txt",
			"dir/file3.txt": "file1.txt",
			"dir/file4.txt": "file2.txt",
			"dir/file5.txt": "file1.txt",
		}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		dirFile, err := logicalFS.Open("dir")
		if err != nil {
			t.Fatal(err)
		}
		defer dirFile.Close()

		readDirFile, ok := dirFile.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected ReadDirFile")
		}

		// Read 2 entries at a time
		entries1, err := readDirFile.ReadDir(2)
		if err != nil {
			t.Fatalf("ReadDir(2) first call: %v", err)
		}
		if len(entries1) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries1))
		}

		// Read 2 more entries
		entries2, err := readDirFile.ReadDir(2)
		if err != nil {
			t.Fatalf("ReadDir(2) second call: %v", err)
		}
		if len(entries2) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries2))
		}

		// Read remaining entries (should be 1 left)
		entries3, err := readDirFile.ReadDir(2)
		if err != nil {
			t.Fatalf("ReadDir(2) third call: %v", err)
		}
		if len(entries3) != 1 {
			t.Errorf("expected 1 entry, got %d", len(entries3))
		}

		// Next call should return EOF
		entries4, err := readDirFile.ReadDir(2)
		if err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}
		if len(entries4) != 0 {
			t.Errorf("expected 0 entries after EOF, got %d",
				len(entries4))
		}

		// Verify all entries are unique
		allNames := make(map[string]bool)
		for _, entries := range [][]fs.DirEntry{
			entries1, entries2, entries3,
		} {
			for _, entry := range entries {
				if allNames[entry.Name()] {
					t.Errorf("duplicate entry name: %s",
						entry.Name())
				}
				allNames[entry.Name()] = true
			}
		}
		if len(allNames) != 5 {
			t.Errorf("expected 5 unique entries, got %d",
				len(allNames))
		}
	})

	t.Run("readdir all at once", func(t *testing.T) {
		refs := map[string]string{
			"dir/file1.txt": "file1.txt",
			"dir/file2.txt": "file2.txt",
			"dir/file3.txt": "file1.txt",
		}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		dirFile, err := logicalFS.Open("dir")
		if err != nil {
			t.Fatal(err)
		}
		defer dirFile.Close()

		readDirFile, ok := dirFile.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected ReadDirFile")
		}

		// ReadDir with n <= 0 should return all entries
		entries, err := readDirFile.ReadDir(0)
		if err != nil {
			t.Fatalf("ReadDir(0): %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries))
		}

		// Subsequent call should return empty and no error
		entries2, err := readDirFile.ReadDir(0)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		if len(entries2) != 0 {
			t.Errorf("expected 0 entries on second call, got %d",
				len(entries2))
		}
	})

	t.Run("readdir negative n", func(t *testing.T) {
		refs := map[string]string{
			"dir/file1.txt": "file1.txt",
			"dir/file2.txt": "file2.txt",
		}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		dirFile, err := logicalFS.Open("dir")
		if err != nil {
			t.Fatal(err)
		}
		defer dirFile.Close()

		readDirFile, ok := dirFile.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected ReadDirFile")
		}

		// ReadDir with negative n should return all entries
		entries, err := readDirFile.ReadDir(-1)
		if err != nil {
			t.Fatalf("ReadDir(-1): %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
	})

	t.Run("dir entry unused methods", func(t *testing.T) {
		// Test the Mode(), ModTime(), and Sys() methods on
		// logicalDirEntry, even though they're not used
		refs := map[string]string{"dir/file1.txt": "file1.txt"}
		created := time.Now()
		logicalFS := logical.NewLogicalFS(ctx, baseFS, refs, created)

		dirFile, err := logicalFS.Open(".")
		if err != nil {
			t.Fatal(err)
		}
		defer dirFile.Close()

		readDirFile, ok := dirFile.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected ReadDirFile")
		}

		entries, err := readDirFile.ReadDir(-1)
		if err != nil {
			t.Fatal(err)
		}

		if len(entries) == 0 {
			t.Fatal("expected at least one entry")
		}

		entry := entries[0]
		// These methods exist but aren't part of the DirEntry interface
		// We need to use reflection or type assertion to access them
		type extendedEntry interface {
			fs.DirEntry
			Mode() fs.FileMode
			ModTime() time.Time
			Sys() any
		}

		if ext, ok := entry.(extendedEntry); ok {
			mode := ext.Mode()
			if !mode.IsDir() {
				t.Error("expected directory mode")
			}

			modtime := ext.ModTime()
			if !modtime.Equal(created) {
				t.Error("modtime mismatch")
			}

			if ext.Sys() != nil {
				t.Error("expected Sys() to return nil")
			}
		}
	})
}

func testOCFLFS(content map[string]string) ocflfs.FS {
	testData := make(fstest.MapFS, len(content))
	for name, cont := range content {
		testData[name] = &fstest.MapFile{Data: []byte(cont)}
	}
	return ocflfs.NewWrapFS(testData)
}
