package ocfl_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
)

func TestFiles(t *testing.T) {
	type testCase struct {
		desc   string
		fsys   ocfl.FS
		dir    string
		expect func(*testing.T, ocfl.FileSeq)
	}
	tests := []testCase{
		{
			desc: "basic",
			dir:  ".",
			fsys: ocfl.NewFS(fstest.MapFS{
				"file.txt": &fstest.MapFile{Data: []byte("content")},
			}),
			expect: func(t *testing.T, files ocfl.FileSeq) {
				files(func(info ocfl.FileInfo, err error) bool {
					be.NilErr(t, err)
					be.Equal(t, "file.txt", info.Path)
					be.Equal(t, len("content"), int(info.Size))
					return true
				})
			},
		}, {
			desc: "deep",
			dir:  "a",
			fsys: ocfl.NewFS(fstest.MapFS{
				"file.txt":         &fstest.MapFile{Data: []byte("content")},
				"a/file.txt":       &fstest.MapFile{Data: []byte("content")},
				"a/b/file.txt":     &fstest.MapFile{Data: []byte("content")},
				"a/b/c/file.txt":   &fstest.MapFile{Data: []byte("content")},
				"a/b/c/d/file.txt": &fstest.MapFile{Data: []byte("content")},
			}),
			expect: func(t *testing.T, files ocfl.FileSeq) {
				count := 0
				files(func(info ocfl.FileInfo, err error) bool {
					be.NilErr(t, err)
					be.True(t, strings.HasPrefix(info.Path, "a/"))
					count++
					return true
				})
				be.Equal(t, 4, count)
			},
		}, {
			desc: "irregular file type ok",
			dir:  ".",
			fsys: ocfl.NewFS(fstest.MapFS{
				"file.txt": &fstest.MapFile{
					Data: []byte("content"),
					Mode: fs.ModeIrregular,
				},
			}),
			expect: func(t *testing.T, files ocfl.FileSeq) {
				files(func(info ocfl.FileInfo, err error) bool {
					be.NilErr(t, err)
					return true
				})
			},
		}, {
			desc: "use  FilesFS implementation",
			dir:  ".",
			fsys: &mockFilesFS{},
			expect: func(t *testing.T, files ocfl.FileSeq) {
				files(func(info ocfl.FileInfo, err error) bool {
					be.NilErr(t, err)
					return true
				})
			},
		}, {
			desc: "empty path is error",
			dir:  "",
			fsys: ocfl.NewFS(fstest.MapFS{
				"file.txt": &fstest.MapFile{Data: []byte("content")},
			}),
			expect: func(t *testing.T, files ocfl.FileSeq) {
				files(func(info ocfl.FileInfo, err error) bool {
					be.Nonzero(t, err)
					be.True(t, errors.Is(err, fs.ErrInvalid))
					return true
				})
			},
		}, {
			desc: "invalid path is error",
			dir:  "../tmp",
			fsys: ocfl.NewFS(fstest.MapFS{
				"file.txt": &fstest.MapFile{Data: []byte("content")},
			}),
			expect: func(t *testing.T, files ocfl.FileSeq) {
				files(func(info ocfl.FileInfo, err error) bool {
					be.Nonzero(t, err)
					be.True(t, errors.Is(err, fs.ErrInvalid))
					return true
				})
			},
		}, {
			desc: "invalid file types",
			dir:  ".",
			fsys: ocfl.NewFS(fstest.MapFS{
				"symlink": &fstest.MapFile{Mode: fs.ModeSymlink},
				"device":  &fstest.MapFile{Mode: fs.ModeDevice},
				"file":    &fstest.MapFile{Mode: fs.ModeSocket},
				"pipe":    &fstest.MapFile{Mode: fs.ModeNamedPipe},
			}),
			expect: func(t *testing.T, files ocfl.FileSeq) {
				files(func(info ocfl.FileInfo, err error) bool {
					be.Nonzero(t, err)
					be.True(t, errors.Is(err, ocfl.ErrFileType))
					return true
				})
			},
		},
	}
	for i, tcase := range tests {
		t.Run(fmt.Sprintf("%d-%s", i, tcase.desc), func(t *testing.T) {
			ctx := context.Background()
			tcase.expect(t, ocfl.Files(ctx, tcase.fsys, tcase.dir))
		})
	}
}

type mockFilesFS struct{}

var _ ocfl.FilesFS = (*mockFilesFS)(nil)

func (m *mockFilesFS) OpenFile(_ context.Context, _ string) (fs.File, error) {
	return nil, errors.New("shouldn't be called")
}

func (m *mockFilesFS) ReadDir(_ context.Context, _ string) ([]fs.DirEntry, error) {
	return nil, errors.New("shouldn't be called")
}

func (m *mockFilesFS) Files(_ context.Context, _ string) ocfl.FileSeq {
	return func(yield func(ocfl.FileInfo, error) bool) {
		yield(ocfl.FileInfo{Path: "test", Size: 1}, nil)
	}
}
