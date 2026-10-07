package ocfl_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/config"
	"github.com/srerickson/ocfl-go/fs/local"
)

func TestContentMap(t *testing.T) {
	ctx := context.Background()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	t.Run("zero value", func(t *testing.T) {
		var c ocfl.ContentMap
		srcFS, srcPath := c.GetContent(digA)
		be.Zero(t, srcFS)
		be.Zero(t, srcPath)
		be.Equal(t, 0, len(c.Digests()))
	})
	t.Run("files and bytes", func(t *testing.T) {
		fsys := ocflfs.DirFS(`testdata`)
		var c ocfl.ContentMap
		c.AddFile(strings.ToUpper(digA), fsys, "content-fixture/hello.csv", nil)
		c.AddBytes(digB, []byte("content"))
		be.DeepEqual(t, []string{digA, digB}, c.Digests())
		srcFS, srcPath := c.GetContent(digA)
		be.Equal[ocflfs.FS](t, fsys, srcFS)
		be.Equal(t, "content-fixture/hello.csv", srcPath)
		srcFS, srcPath = c.GetContent(digB)
		be.Nonzero(t, srcFS)
		got, err := ocflfs.ReadAll(ctx, srcFS, srcPath)
		be.NilErr(t, err)
		be.Equal(t, "content", string(got))
		// replace bytes with a file
		c.AddFile(digB, fsys, "content-fixture/folder1/file.txt", nil)
		srcFS, srcPath = c.GetContent(digB)
		be.Equal[ocflfs.FS](t, fsys, srcFS)
		be.Equal(t, "content-fixture/folder1/file.txt", srcPath)
		c.Remove(digA)
		be.DeepEqual(t, []string{digB}, c.Digests())
		srcFS, _ = c.GetContent(digA)
		be.Zero(t, srcFS)
	})
	t.Run("FS values that can't be compared", func(t *testing.T) {
		// the struct type is comparable, but its value isn't
		fsys := mapFS{files: fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("a")}}}
		var c ocfl.ContentMap
		c.AddFile(digA, fsys, "a.txt", nil)
		c.AddFile(digB, fsys, "a.txt", nil)
		srcFS, srcPath := c.GetContent(digB)
		got, err := ocflfs.ReadAll(ctx, srcFS, srcPath)
		be.NilErr(t, err)
		be.Equal(t, "a", string(got))
	})
}

// mapFS is an ocflfs.FS with a field that holds a map.
type mapFS struct{ files fs.FS }

func (m mapFS) OpenFile(_ context.Context, name string) (fs.File, error) {
	return m.files.Open(name)
}

func TestContentMap_JSON(t *testing.T) {
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	digC := strings.Repeat("c", 128)
	dirA, dirB := t.TempDir(), t.TempDir()
	fsA, err := local.NewFS(dirA)
	be.NilErr(t, err)
	fsB, err := local.NewFS(dirB)
	be.NilErr(t, err)
	fsA2, err := local.NewFS(dirA) // same directory as fsA
	be.NilErr(t, err)
	unused, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)

	var c ocfl.ContentMap
	c.AddFile(digC, unused, "c.txt", fileInfo{size: 3, token: "stat:1:1"})
	c.AddFile(digA, fsA, "dir/a.txt", fileInfo{size: 10, token: "stat:1:2"})
	c.AddFile(digB, fsB, "b.txt", fileInfo{size: 0}) // token unknown
	c.AddFile(digC, fsA2, "c.txt", nil)              // size and token unknown
	saved, err := json.Marshal(c)
	be.NilErr(t, err)
	// unused sources aren't saved, and sources with the same text are saved
	// once.
	var savedJSON struct {
		Sources []string
		Content map[string][]any
	}
	be.NilErr(t, json.Unmarshal(saved, &savedJSON))
	be.Equal(t, 2, len(savedJSON.Sources))
	// unknown sizes and tokens are saved as null
	be.DeepEqual(t, []any{float64(0), "dir/a.txt", float64(10), "stat:1:2"}, savedJSON.Content[digA])
	be.DeepEqual(t, []any{float64(1), "b.txt", float64(0), nil}, savedJSON.Content[digB])
	be.DeepEqual(t, []any{float64(0), "c.txt", nil, nil}, savedJSON.Content[digC])

	t.Run("load and save without opening", func(t *testing.T) {
		var loaded ocfl.ContentMap
		be.NilErr(t, json.Unmarshal(saved, &loaded))
		be.DeepEqual(t, c.Digests(), loaded.Digests())
		// content isn't available until the sources are opened
		for _, dig := range loaded.Digests() {
			srcFS, srcPath := loaded.GetContent(dig)
			be.Zero(t, srcFS)
			be.Zero(t, srcPath)
		}
		resaved, err := json.Marshal(loaded)
		be.NilErr(t, err)
		be.Equal(t, string(saved), string(resaved))
	})
	t.Run("in a struct", func(t *testing.T) {
		type file struct {
			Name    string           `json:"name"`
			Content *ocfl.ContentMap `json:"content"`
		}
		data, err := json.Marshal(file{Name: "test", Content: &c})
		be.NilErr(t, err)
		var loaded file
		be.NilErr(t, json.Unmarshal(data, &loaded))
		be.Equal(t, "test", loaded.Name)
		be.DeepEqual(t, c.Digests(), loaded.Content.Digests())
	})
	t.Run("add after loading", func(t *testing.T) {
		var loaded ocfl.ContentMap
		be.NilErr(t, json.Unmarshal(saved, &loaded))
		digD := strings.Repeat("d", 128)
		digE := strings.Repeat("e", 128)
		dirC := t.TempDir()
		fsC, err := local.NewFS(dirC)
		be.NilErr(t, err)
		loaded.AddFile(digD, fsC, "d.txt", fileInfo{size: 4, token: "etag:d"})
		// a directory that was loaded is saved once
		loaded.AddFile(digE, fsA2, "e.txt", nil)
		resaved, err := json.Marshal(loaded)
		be.NilErr(t, err)
		var resavedJSON struct {
			Sources []string
			Content map[string][]any
		}
		be.NilErr(t, json.Unmarshal(resaved, &resavedJSON))
		be.DeepEqual(t, append(savedJSON.Sources, localText(t, dirC)), resavedJSON.Sources)
		be.DeepEqual(t, []any{float64(2), "d.txt", float64(4), "etag:d"}, resavedJSON.Content[digD])
		be.DeepEqual(t, []any{float64(0), "e.txt", nil, nil}, resavedJSON.Content[digE])
	})
	t.Run("bytes can't be saved", func(t *testing.T) {
		var c ocfl.ContentMap
		c.AddBytes(digA, []byte("a"))
		_, err := json.Marshal(c)
		be.Nonzero(t, err)
	})
	t.Run("FS without MarshalText can't be saved", func(t *testing.T) {
		var c ocfl.ContentMap
		c.AddFile(digA, ocflfs.DirFS(dirA), "a.txt", nil)
		_, err := json.Marshal(c)
		be.Nonzero(t, err)
	})
	t.Run("invalid saved values", func(t *testing.T) {
		entry := func(e string) string {
			return `{"sources": ["file:///a"], "content": {"` + digA + `": ` + e + `}}`
		}
		for name, data := range map[string]string{
			"bad index":      entry(`[1, "a.txt", null, null]`),
			"bad path":       entry(`[0, "../a.txt", null, null]`),
			"two elements":   entry(`[0, "a.txt"]`),
			"three elements": entry(`[0, "a.txt", 1]`),
			"five elements":  entry(`[0, "a.txt", 1, "etag:a", 2]`),
			"negative size":  entry(`[0, "a.txt", -1, null]`),
			"bad size":       entry(`[0, "a.txt", "1", null]`),
			"empty token":    entry(`[0, "a.txt", 1, ""]`),
			"bad token":      entry(`[0, "a.txt", 1, 2]`),
			"empty source":   `{"sources": [""], "content": {}}`,
			"unknown field":  `{"sources": [], "content": {}, "extra": 1}`,
		} {
			t.Run(name, func(t *testing.T) {
				var c ocfl.ContentMap
				be.Nonzero(t, json.Unmarshal([]byte(data), &c))
			})
		}
	})
}

func TestContentMap_Open(t *testing.T) {
	ctx := context.Background()
	reg := config.Registry()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	// load returns a ContentMap loaded from JSON with content in two local
	// directories, and the directories.
	load := func(t *testing.T) (*ocfl.ContentMap, string, string) {
		t.Helper()
		dirA, dirB := t.TempDir(), t.TempDir()
		data := `{"sources": [` + string(mustMarshal(t, localText(t, dirA))) + `, ` + string(mustMarshal(t, localText(t, dirB))) + `],
			"content": {"` + digA + `": [0, "a.txt", null, null], "` + digB + `": [1, "b.txt", null, null]}}`
		var c ocfl.ContentMap
		be.NilErr(t, json.Unmarshal([]byte(data), &c))
		return &c, dirA, dirB
	}
	t.Run("open", func(t *testing.T) {
		c, dirA, dirB := load(t)
		be.NilErr(t, c.OpenFS(ctx, reg))
		srcFS, srcPath := c.GetContent(digA)
		be.Equal(t, dirA, srcFS.(*local.FS).Root())
		be.Equal(t, "a.txt", srcPath)
		srcFS, srcPath = c.GetContent(digB)
		be.Equal(t, dirB, srcFS.(*local.FS).Root())
		be.Equal(t, "b.txt", srcPath)
		// opening again doesn't open sources again
		be.NilErr(t, c.OpenFS(ctx, ocflfs.Registry{}))
		gotFS, _ := c.GetContent(digA)
		be.Equal(t, dirA, gotFS.(*local.FS).Root())
	})
	t.Run("missing directory", func(t *testing.T) {
		c, dirA, dirB := load(t)
		textA := localText(t, dirA)
		be.NilErr(t, os.Remove(dirA))
		err := c.OpenFS(ctx, reg)
		be.True(t, errors.Is(err, fs.ErrNotExist))
		be.In(t, textA, err.Error())
		// the other source was opened
		srcFS, _ := c.GetContent(digB)
		be.Equal(t, dirB, srcFS.(*local.FS).Root())
		srcFS, _ = c.GetContent(digA)
		be.Zero(t, srcFS)
	})
	t.Run("removed content isn't opened", func(t *testing.T) {
		c, dirA, _ := load(t)
		be.NilErr(t, os.Remove(dirA))
		c.Remove(digA)
		be.NilErr(t, c.OpenFS(ctx, reg))
	})
	t.Run("unknown scheme", func(t *testing.T) {
		c, _, _ := load(t)
		err := c.OpenFS(ctx, ocflfs.Registry{})
		be.True(t, errors.Is(err, ocflfs.ErrUnknownScheme))
	})
	t.Run("passwords aren't in errors", func(t *testing.T) {
		data := `{"sources": ["https://user:secret@example.org/ocfl"], "content": {"` + digA + `": [0, "a.txt", null, null]}}`
		var c ocfl.ContentMap
		be.NilErr(t, json.Unmarshal([]byte(data), &c))
		err := c.OpenFS(ctx, ocflfs.Registry{})
		be.Nonzero(t, err)
		be.NotIn(t, "secret", err.Error())
	})
}

func TestContentMap_Check(t *testing.T) {
	ctx := context.Background()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	digC := strings.Repeat("c", 128)
	digD := strings.Repeat("d", 128)
	digE := strings.Repeat("e", 128)
	// setup returns a ContentMap with content in files in a local directory,
	// the directory, and the files' content tokens when they were added. d.txt
	// is added without a size or token. The files have old modification
	// times, so that rewriting them changes their tokens.
	setup := func(t *testing.T) (*ocfl.ContentMap, string, map[string]string) {
		t.Helper()
		dir := t.TempDir()
		for name, data := range map[string]string{
			"a.txt": "aaa", "b.txt": "bb", "c.txt": "c", "d.txt": "dddd",
		} {
			writeOld(t, filepath.Join(dir, name), data)
		}
		fsys, err := local.NewFS(dir)
		be.NilErr(t, err)
		c := &ocfl.ContentMap{}
		tokens := map[string]string{}
		for dig, name := range map[string]string{digA: "a.txt", digB: "b.txt", digC: "c.txt"} {
			info, err := ocflfs.StatFile(ctx, fsys, name)
			be.NilErr(t, err)
			tokens[name] = ocflfs.ContentToken(info)
			be.Nonzero(t, tokens[name])
			c.AddFile(dig, fsys, name, info)
		}
		c.AddFile(digD, fsys, "d.txt", nil)
		c.AddBytes(digE, []byte("in memory"))
		be.NilErr(t, c.Check(ctx))
		return c, dir, tokens
	}
	// changes returns the changes in err, without their FSs.
	changes := func(t *testing.T, err error) []ocfl.ContentChange {
		t.Helper()
		be.True(t, errors.Is(err, ocfl.ErrContentChanged))
		var changedErr *ocfl.ContentChangedError
		be.True(t, errors.As(err, &changedErr))
		got := changedErr.Changes
		for i := range got {
			got[i].FS = nil
		}
		return got
	}
	t.Run("changed and missing files", func(t *testing.T) {
		c, dir, tokens := setup(t)
		be.NilErr(t, os.Truncate(filepath.Join(dir, "a.txt"), 1))
		be.NilErr(t, os.Remove(filepath.Join(dir, "b.txt")))
		// d.txt's size and token aren't checked
		be.NilErr(t, os.WriteFile(filepath.Join(dir, "d.txt"), []byte("d"), 0o644))
		err := c.Check(ctx)
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digA, Path: "a.txt", Size: 3, Token: tokens["a.txt"], NewSize: 1, NewToken: statToken(t, dir, "a.txt")},
			{Digest: digB, Path: "b.txt", Size: 2, Token: tokens["b.txt"], Missing: true},
		}, changes(t, err))
		be.In(t, `"a.txt" has size 1, not 3`, err.Error())
		be.In(t, `"b.txt" is missing`, err.Error())
		// only the given digests are checked
		be.NilErr(t, c.CheckContent(ctx, []string{digC, digD, digE, strings.Repeat("f", 128)}))
		err = c.CheckContent(ctx, []string{strings.ToUpper(digB)})
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digB, Path: "b.txt", Size: 2, Token: tokens["b.txt"], Missing: true},
		}, changes(t, err))
	})
	t.Run("same size", func(t *testing.T) {
		c, dir, tokens := setup(t)
		be.NilErr(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("xxx"), 0o644))
		newToken := statToken(t, dir, "a.txt")
		err := c.Check(ctx)
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digA, Path: "a.txt", Size: 3, Token: tokens["a.txt"], NewSize: 3, NewToken: newToken},
		}, changes(t, err))
		be.In(t, `"a.txt" has changed since it was added (`+tokens["a.txt"]+` is now `+newToken+`)`, err.Error())
	})
	t.Run("missing file without size", func(t *testing.T) {
		c, dir, _ := setup(t)
		be.NilErr(t, os.Remove(filepath.Join(dir, "d.txt")))
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digD, Path: "d.txt", Size: -1, Missing: true},
		}, changes(t, c.Check(ctx)))
	})
	t.Run("directory in place of a file", func(t *testing.T) {
		c, dir, tokens := setup(t)
		be.NilErr(t, os.Remove(filepath.Join(dir, "c.txt")))
		be.NilErr(t, os.Mkdir(filepath.Join(dir, "c.txt"), 0o755))
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digC, Path: "c.txt", Size: 1, Token: tokens["c.txt"], Missing: true},
		}, changes(t, c.Check(ctx)))
	})
	t.Run("loaded from JSON", func(t *testing.T) {
		c, dir, tokens := setup(t)
		c.Remove(digE)
		var loaded ocfl.ContentMap
		be.NilErr(t, json.Unmarshal(mustMarshal(t, c), &loaded))
		// content in an FS that isn't open is missing
		err := loaded.Check(ctx)
		be.Equal(t, 4, len(changes(t, err)))
		be.NilErr(t, loaded.OpenFS(ctx, config.Registry()))
		be.NilErr(t, loaded.Check(ctx))
		be.NilErr(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("xxx"), 0o644))
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digA, Path: "a.txt", Size: 3, Token: tokens["a.txt"], NewSize: 3, NewToken: statToken(t, dir, "a.txt")},
		}, changes(t, loaded.Check(ctx)))
	})
	t.Run("loaded from JSON without sizes or tokens", func(t *testing.T) {
		dir := t.TempDir()
		be.NilErr(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0o644))
		data := `{"sources": [` + string(mustMarshal(t, localText(t, dir))) + `],
			"content": {"` + digA + `": [0, "a.txt", null, null], "` + digB + `": [0, "b.txt", null, null]}}`
		var c ocfl.ContentMap
		be.NilErr(t, json.Unmarshal([]byte(data), &c))
		be.NilErr(t, c.OpenFS(ctx, config.Registry()))
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digB, Path: "b.txt", Size: -1, Missing: true},
		}, changes(t, c.Check(ctx)))
		// saved without sizes or tokens again
		be.In(t, `[0,"a.txt",null,null]`, string(mustMarshal(t, c)))
	})
	t.Run("unrecognized token", func(t *testing.T) {
		// a token of a kind the backend doesn't produce is a change, not an
		// error
		dir := t.TempDir()
		be.NilErr(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0o644))
		data := `{"sources": [` + string(mustMarshal(t, localText(t, dir))) + `],
			"content": {"` + digA + `": [0, "a.txt", 3, "future:1"]}}`
		var c ocfl.ContentMap
		be.NilErr(t, json.Unmarshal([]byte(data), &c))
		be.NilErr(t, c.OpenFS(ctx, config.Registry()))
		err := c.Check(ctx)
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digA, Path: "a.txt", Size: 3, Token: "future:1", NewSize: 3, NewToken: statToken(t, dir, "a.txt")},
		}, changes(t, err))
		var changedErr *ocfl.ContentChangedError
		be.True(t, errors.As(err, &changedErr))
		be.Equal(t, changedErr.Error(), err.Error())
	})
	t.Run("FS without tokens", func(t *testing.T) {
		// files are checked by size only
		files := fstest.MapFS{"a.txt": {Data: []byte("aaa")}}
		fsys := ocflfs.NewWrapFS(files)
		info, err := ocflfs.StatFile(ctx, fsys, "a.txt")
		be.NilErr(t, err)
		var c ocfl.ContentMap
		c.AddFile(digA, fsys, "a.txt", info)
		files["a.txt"] = &fstest.MapFile{Data: []byte("xxx")}
		be.NilErr(t, c.Check(ctx))
		files["a.txt"] = &fstest.MapFile{Data: []byte("x")}
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digA, Path: "a.txt", Size: 3, NewSize: 1},
		}, changes(t, c.Check(ctx)))
	})
	t.Run("stat errors", func(t *testing.T) {
		var c ocfl.ContentMap
		fsys := &openErrFS{WrapFS: ocflfs.NewWrapFS(fstest.MapFS{"a.txt": {}}), name: "a.txt"}
		c.AddFile(digA, fsys, "a.txt", fileInfo{size: 0})
		c.AddFile(digB, fsys, "b.txt", fileInfo{size: 0})
		err := c.Check(ctx)
		be.True(t, errors.Is(err, errOpen))
		be.DeepEqual(t, []ocfl.ContentChange{
			{Digest: digB, Path: "b.txt", Size: 0, Missing: true},
		}, changes(t, err))
	})
	t.Run("canceled", func(t *testing.T) {
		c, _, _ := setup(t)
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		be.True(t, errors.Is(c.Check(ctx), context.Canceled))
	})
}

func TestContentChange_String(t *testing.T) {
	for name, tc := range map[string]struct {
		change ocfl.ContentChange
		want   string
	}{
		"missing": {
			change: ocfl.ContentChange{Path: "a.txt", Size: 3, Token: "etag:a", Missing: true},
			want:   `"a.txt" is missing`,
		},
		"size and token changed": {
			change: ocfl.ContentChange{Path: "a.txt", Size: 3, Token: "etag:a", NewSize: 1, NewToken: "etag:b"},
			want:   `"a.txt" has size 1, not 3`,
		},
		"token changed": {
			change: ocfl.ContentChange{Path: "a.txt", Size: 3, Token: "etag:a", NewSize: 3, NewToken: "etag:b"},
			want:   `"a.txt" has changed since it was added (etag:a is now etag:b)`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			be.Equal(t, tc.want, tc.change.String())
		})
	}
}

// fileInfo is an fs.FileInfo for a file with a size and content token.
type fileInfo struct {
	size  int64
	token string
}

func (i fileInfo) Name() string         { return "file" }
func (i fileInfo) Size() int64          { return i.size }
func (i fileInfo) Mode() fs.FileMode    { return 0 }
func (i fileInfo) ModTime() time.Time   { return time.Time{} }
func (i fileInfo) IsDir() bool          { return false }
func (i fileInfo) Sys() any             { return nil }
func (i fileInfo) ContentToken() string { return i.token }

// writeOld writes data to the file name and sets its modification time to an
// hour ago, so that rewriting the file changes its content token even on file
// systems with coarse timestamps.
func writeOld(t *testing.T, name, data string) {
	t.Helper()
	be.NilErr(t, os.WriteFile(name, []byte(data), 0o644))
	old := time.Now().Add(-time.Hour)
	be.NilErr(t, os.Chtimes(name, old, old))
}

// statToken returns the content token for the file name in dir.
func statToken(t *testing.T, dir, name string) string {
	t.Helper()
	info, err := os.Stat(filepath.Join(dir, name))
	be.NilErr(t, err)
	return ocflfs.ContentToken(info)
}

// localText returns the text that a local.FS for dir is saved as.
func localText(t *testing.T, dir string) string {
	t.Helper()
	text, err := local.MustNewFS(dir).MarshalText()
	be.NilErr(t, err)
	return string(text)
}
