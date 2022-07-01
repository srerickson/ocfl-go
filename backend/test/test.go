package test

// Test suite for backend.Interface

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl/backend"
)

var (
	// parent directory for the test
	prefix    = fmt.Sprintf("backend-test-%d", time.Now().Unix())
	testFiles = []string{
		"a.txt",
		"a/b.txt",
		"a/b/c.txt",
		"a/b/c/d.txt",
	}
)

// TestBackend is complete test suite for backend.Interface
func TestBackend(t *testing.T, bak backend.Interface) {
	is := is.New(t)

	// confirm buildTestDir works
	is.NoErr(buildTestDir(bak))
	list, err := listFiles(bak)
	is.NoErr(err)
	is.Equal(list, testFiles)

	// actual tests
	TestWrite(t, bak)
	TestRemoveAll(t, bak)
	TestCopy(t, bak)

	// cleanup
	is.NoErr(bak.RemoveAll(prefix))
}

func buildTestDir(bak backend.Interface) error {
	if err := bak.RemoveAll(prefix); err != nil {
		return err
	}
	// write test content
	for _, f := range testFiles {
		f = path.Join(prefix, f)
		_, err := bak.Write(f, strings.NewReader(f))
		if err != nil {
			return fmt.Errorf("creating test files: %w", err)
		}
	}
	return nil
}

func listFiles(bak backend.Interface) ([]string, error) {
	var files []string
	err := fs.WalkDir(bak, prefix, func(p string, inf fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if inf.Type().IsRegular() {
			p := strings.TrimPrefix(p, prefix+"/")
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		pErr, ok := err.(*fs.PathError)
		if !ok {
			return nil, err
		}
		// ok if prefix dir doesn't exist
		if strings.HasSuffix(pErr.Path, prefix) && errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func TestWrite(t *testing.T, bak backend.Interface) {
	type tableEntry struct {
		desc   string
		name   string
		before []string
		after  []string
		err    bool
	}
	table := []tableEntry{
		{desc: "existing file",
			name: "a.txt", before: testFiles, after: testFiles, err: false},
		{desc: "new file",
			name: "a2.txt", before: testFiles, after: append(testFiles, "a2.txt"), err: false},
		{desc: "new file in subdir",
			name: "a/b/c/d/e/f.txt", before: testFiles, after: append(testFiles, "a/b/c/d/e/f.txt"), err: false},
		{desc: "existing directory",
			name: "a/b/c", before: testFiles, after: testFiles, err: true},
		{desc: "parent",
			name: "..", before: testFiles, after: testFiles, err: true},
		{desc: "invalid path",
			name: "a/../a2.txt", before: testFiles, after: testFiles, err: true},
	}
	for _, e := range table {
		t.Run("write: "+e.desc, func(t *testing.T) {
			is := is.New(t)
			is.NoErr(buildTestDir(bak))

			before, err := listFiles(bak)
			is.NoErr(err)
			is.Equal(before, e.before)

			cont := "new contents-" + e.name
			buff := strings.NewReader(cont)
			f := prefix + "/" + e.name
			_, err = bak.Write(f, buff)
			if e.err {
				is.True(err != nil)
			} else {
				is.NoErr(err)
			}
			// confirm contents of file if we expected successful write
			if !e.err {
				byt, err := fs.ReadFile(bak, f)
				is.NoErr(err)
				is.Equal(string(byt), cont)
			}
			after, err := listFiles(bak)
			is.NoErr(err)
			is.Equal(after, e.after)
		})
	}
}

func TestRemoveAll(t *testing.T, bak backend.Interface) {
	type tableEntry struct {
		name   string
		before []string
		after  []string
		err    bool
	}
	table := []tableEntry{
		// file that exists - no error
		{name: "a.txt", before: testFiles, after: []string{"a/b.txt", "a/b/c.txt", "a/b/c/d.txt"}, err: false},
		// file that does not exist - no error
		{name: "a2.txt", before: testFiles, after: testFiles, err: false},
		// directory that exists - no error
		{name: "a/b", before: testFiles, after: []string{"a.txt", "a/b.txt"}, err: false},
		{name: "a", before: testFiles, after: []string{"a.txt"}, err: false},
		// directory that doesn't exist - no error
		{name: "a2", before: testFiles, after: testFiles, err: false},
		// errors:
		{name: "../a.txt", before: testFiles, after: testFiles, err: true},
		{name: "a/../a.txt", before: testFiles, after: testFiles, err: true},
	}
	for _, e := range table {
		t.Run("removeAll: "+e.name, func(t *testing.T) {
			is := is.New(t)
			is.NoErr(buildTestDir(bak))

			before, err := listFiles(bak)
			is.NoErr(err)
			is.Equal(before, e.before)

			f := prefix + "/" + e.name
			err = bak.RemoveAll(f)
			if e.err {
				is.True(err != nil) // expecting an error
			} else {
				is.NoErr(err) // expect no error
			}
			// confirm contents of file if we expected successful write
			_, err = fs.Stat(bak, f)
			if !e.err {
				is.True(errors.Is(err, fs.ErrNotExist))
			}
			after, err := listFiles(bak)
			is.NoErr(err)
			is.Equal(after, e.after)
		})
	}
	t.Run("removeAll: root", func(t *testing.T) {
		is := is.New(t)
		err := bak.RemoveAll(".")
		is.True(err != nil) // RemoveAll "." should fail
	})
}

func TestCopy(t *testing.T, bak backend.Interface) {
	type tableEntry struct {
		desc   string
		src    string
		dst    string
		before []string
		after  []string
		err    bool
	}
	table := []tableEntry{
		{desc: "copy existing file to new file",
			src: "a.txt", dst: "a2.txt", before: testFiles, after: append(testFiles, "a2.txt"), err: false},
		{desc: "copy existing file to exsting file",
			src: "a.txt", dst: "a/b.txt", before: testFiles, after: testFiles, err: false},
		{desc: "copy existing file to exsting directory",
			src: "a.txt", dst: "a/b/c", before: testFiles, after: testFiles, err: true},
		{desc: "copy existing file to invalid path",
			src: "a.txt", dst: "a/../a2.txt", before: testFiles, after: testFiles, err: true},
		{desc: "copy missing file",
			src: "z.txt", dst: "a/b.txt", before: testFiles, after: testFiles, err: true},
		{desc: "copy dir: a/b",
			src: "a/b", dst: "a/b2", before: testFiles, after: testFiles, err: true},
		{desc: "copy dir:.",
			src: ".", dst: "2", before: testFiles, after: testFiles, err: true},
		{desc: "copy invalid path",
			src: "a/../a.txt", dst: "a3.txt", before: testFiles, after: testFiles, err: true},
	}
	for _, e := range table {
		t.Run("copy: "+e.desc, func(t *testing.T) {
			is := is.New(t)
			is.NoErr(buildTestDir(bak))

			before, err := listFiles(bak)
			is.NoErr(err)
			is.Equal(before, e.before)

			src := prefix + "/" + e.src
			dst := prefix + "/" + e.dst

			err = bak.Copy(dst, src)
			if e.err {
				// expecting an error
				is.True(err != nil)
			} else {
				// expect no error
				is.NoErr(err)
			}
			// confirm contents of file if we expected successful write
			_, err = fs.Stat(bak, dst)
			if !e.err {
				is.NoErr(err)
			}
			after, err := listFiles(bak)
			is.NoErr(err)
			is.Equal(after, e.after)
		})
	}
}
