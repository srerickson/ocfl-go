package s3_test

// Tests for direntries.go: BucketFS.DirEntries and the ocflfs.ReadDir helper
// built on it, including the shared cross-backend DirEntries contract.

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"sort"
	"strconv"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// These tests pin the exact behavior of s3.dirEntries() (direntries.go) for
// prefixes that have no objects:
//
//   - A prefix with no objects and no common prefixes is treated as a missing
//     directory: dirEntries yields fs.ErrNotExist, via its
//     `prefixHasContent` guard.
//   - A prefix that exists only as a common prefix (object keys are nested
//     one level deeper) is NOT empty: the CommonPrefixes alone count as
//     content, and are yielded as subdirectory entries.
//   - dir="." on an empty bucket is the one exception to the missing-directory
//     rule: no Prefix is set and the first page is empty,
//     but "." names the bucket itself, which always exists (a missing bucket
//     surfaces as a ListObjectsV2 error), so it yields zero entries and no
//     error — the same as the local backend's readdir of an existing but
//     empty directory (fs/local/localfs_test.go, "empty top-level directory
//     returns zero entries"). An empty bucket is a valid (new) OCFL storage
//     root and must read back as empty for Root.NewRoot and
//     ocflfs.RemoveAll(".").
//
// Callers: BucketFS.DirEntries (fs/s3/fs.go:78) -- errors propagate verbatim
// through ocflfs.ReadDir (fs/fs.go:137).
func TestDirEntries_PrefixNoContent_ErrNotExist(t *testing.T) {
	// Case (1): dir "missing" has no objects and no common prefixes, even
	// though the bucket itself is non-empty.
	api := mock.New(bucket,
		&mock.Object{Key: "tmp/test-file-0.txt"},
		&mock.Object{Key: "tmp/dir-0/tmp.txt"},
	)
	fsys := s3.NewBucketFS(api, bucket)
	ctx := context.Background()

	entries, err := ocflfs.ReadDir(ctx, fsys, "missing")
	be.Zero(t, entries)
	be.Nonzero(t, err)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "readdir", pathErr.Op)
	be.Equal(t, "missing", pathErr.Path)
	be.True(t, errors.Is(err, fs.ErrNotExist))

	// The raw iterator must yield exactly one (nil, err) pair.
	count := 0
	for entry, itErr := range fsys.DirEntries(ctx, "missing") {
		count++
		be.Zero(t, entry)
		be.True(t, errors.Is(itErr, fs.ErrNotExist))
	}
	be.Equal(t, 1, count)
}

func TestDirEntries_CommonPrefixOnly(t *testing.T) {
	// Case (2): "dir" exists only as a common prefix -- all objects are
	// nested below it, so a listing of "dir" has zero Contents and exactly
	// one CommonPrefix. dirEntries must return the subdirectory, not
	// fs.ErrNotExist.
	api := mock.New(bucket,
		&mock.Object{Key: "dir/sub/file.txt"},
		&mock.Object{Key: "dir/sub/deeper/other.txt"},
	)
	fsys := s3.NewBucketFS(api, bucket)
	ctx := context.Background()

	entries, err := ocflfs.ReadDir(ctx, fsys, "dir")
	be.NilErr(t, err)
	be.Equal(t, 1, len(entries))
	entry := entries[0]
	be.Equal(t, "sub", entry.Name())
	be.True(t, entry.IsDir())
	be.Equal(t, fs.ModeDir, entry.Type()&fs.ModeDir)

	// Same shape one level down, where "sub" holds a file *and* a deeper
	// common prefix: both appear, sorted by name (file "file.txt" and
	// subdirectory "deeper").
	entries, err = ocflfs.ReadDir(ctx, fsys, "dir/sub")
	be.NilErr(t, err)
	be.Equal(t, 2, len(entries))
	be.Equal(t, "deeper", entries[0].Name())
	be.True(t, entries[0].IsDir())
	be.Equal(t, "file.txt", entries[1].Name())
	be.True(t, !entries[1].IsDir())
}

func TestDirEntries_RootEmptyBucket_Empty(t *testing.T) {
	// Case (3): dir="." on a bucket with no objects at all. No Prefix is set
	// and the first page is empty, so dirEntries yields
	// zero entries and no error -- never fs.ErrNotExist. The root always
	// exists (it is the bucket itself), so an empty bucket reads as an
	// empty directory, matching the local backend (localfs_test.go, "empty
	// top-level directory returns zero entries") and keeping both
	// Root.NewRoot and ocflfs.RemoveAll(".") working on a fresh bucket.
	api := mock.New(bucket)
	fsys := s3.NewBucketFS(api, bucket)
	ctx := context.Background()

	entries, err := ocflfs.ReadDir(ctx, fsys, ".")
	be.NilErr(t, err)
	be.Zero(t, entries)

	// The raw iterator yields zero (entry, err) pairs, exactly like the
	// local backend's iterator on an empty directory.
	count := 0
	for entry, itErr := range fsys.DirEntries(ctx, ".") {
		count++
		be.Zero(t, entry)
		be.NilErr(t, itErr)
	}
	be.Equal(t, 0, count)
}

func TestDirEntries_RootMissingBucket_ListError(t *testing.T) {
	// Contrast for case (3): "." reads as empty only because the bucket
	// itself exists. Pointing the FS at a bucket that does not exist
	// surfaces the ListObjectsV2 error (typed NoSuchBucket) verbatim --
	// never an empty listing and never fs.ErrNotExist, which would make the
	// root look like an ordinary missing (non-root) directory.
	api := mock.New(bucket)
	fsys := s3.NewBucketFS(api, "no-such-bucket")
	ctx := context.Background()

	entries, err := ocflfs.ReadDir(ctx, fsys, ".")
	be.Zero(t, entries)
	be.Nonzero(t, err)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "readdir", pathErr.Op)
	be.Equal(t, ".", pathErr.Path)
	be.True(t, !errors.Is(err, fs.ErrNotExist))
}

func TestDirEntries_RootEmptyBucket_Integration(t *testing.T) {
	// Live-store counterpart of TestDirEntries_RootEmptyBucket_Empty: a
	// freshly created (empty) bucket must read dir="." as an empty
	// directory with no error, never fs.ErrNotExist. Skipped unless
	// $OCFL_TEST_S3 points at a running store (see internal/testutil).
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running: set $OCFL_TEST_S3 to enable")
	}
	ctx := context.Background()
	// TmpS3FS creates a random empty test bucket and removes it (with all
	// objects) in a t.Cleanup callback.
	fsys := testutil.TmpS3FS(t, nil)

	entries, err := ocflfs.ReadDir(ctx, fsys, ".")
	be.NilErr(t, err)
	be.Zero(t, entries)

	count := 0
	for entry, itErr := range fsys.DirEntries(ctx, ".") {
		count++
		be.Zero(t, entry)
		be.NilErr(t, itErr)
	}
	be.Equal(t, 0, count)
}

func TestDirEntries_RootNonEmptyBucket(t *testing.T) {
	// Contrast for case (3): "." on a bucket that is not empty lists
	// normally, mixing file and common-prefix (subdirectory) entries sorted
	// by name, and never yields ErrNotExist.
	api := mock.New(bucket,
		&mock.Object{Key: "b/c.txt"},
		&mock.Object{Key: "a.txt"},
	)
	fsys := s3.NewBucketFS(api, bucket)
	ctx := context.Background()

	entries, err := ocflfs.ReadDir(ctx, fsys, ".")
	be.NilErr(t, err)
	be.Equal(t, 2, len(entries))
	be.Equal(t, "a.txt", entries[0].Name())
	be.True(t, !entries[0].IsDir())
	be.Equal(t, "b", entries[1].Name())
	be.True(t, entries[1].IsDir())
}

func TestReadDir(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	fixtureFS := ocflfs.DirFS(fixtures)
	fsys := testutil.TmpS3FS(t, fixtureFS)
	type test struct {
		ctx    context.Context
		name   string
		expect func(*testing.T, iter.Seq2[fs.DirEntry, error])
	}

	tests := map[string]test{
		"root": {
			name: ".",
			expect: func(t *testing.T, entries iter.Seq2[fs.DirEntry, error]) {
				ctx := context.Background()
				comparDirEntries(t, entries, ocflfs.DirEntries(ctx, fixtureFS, "."))
			},
		},
		"folder1": {
			name: "folder1",
			expect: func(t *testing.T, entries iter.Seq2[fs.DirEntry, error]) {
				ctx := context.Background()
				comparDirEntries(t, entries, ocflfs.DirEntries(ctx, fixtureFS, "folder1"))
			},
		},
		"missing": {
			name: "missing-dir",
			expect: func(t *testing.T, s iter.Seq2[fs.DirEntry, error]) {
				count := 0
				for entry, err := range s {
					count++
					be.Nonzero(t, err)
					be.True(t, errors.Is(err, fs.ErrNotExist))
					be.Zero(t, entry)
				}
				be.Equal(t, 1, count)
			},
		},
	}
	for desc, test := range tests {
		t.Run(desc, func(t *testing.T) {
			ctx := test.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			test.expect(t, fsys.DirEntries(ctx, test.name))
		})
	}
}

func TestReadDir_Mock(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		bucket string
		dir    string
		mock   func(*testing.T) *mock.S3API
		expect func(*testing.T, []fs.DirEntry, error)
	}
	cases := []testCase{
		{
			desc: "invalid dir",
			dir:  "..",
			expect: func(t *testing.T, _ []fs.DirEntry, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc:   "ErrNotExist",
			bucket: bucket,
			dir:    "missing",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, mock.DirectoryList(10, 0, "tmp/test")...)
			},
			expect: func(t *testing.T, entries []fs.DirEntry, err error) {
				isPathError(t, err)
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		}, {
			desc:   "big directory",
			bucket: bucket,
			dir:    "tmp",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, mock.DirectoryList(1500, 1501, "tmp/test")...)
			},
			expect: func(t *testing.T, entries []fs.DirEntry, err error) {
				be.NilErr(t, err)
				numFiles, numDirs := 0, 0
				for _, entry := range entries {
					info, err := entry.Info()
					be.NilErr(t, err)
					be.Nonzero(t, info.Name())
					be.Nonzero(t, entry.Name())
					switch {
					case entry.IsDir():
						numDirs++
					default:
						numFiles++
					}
				}
				be.Equal(t, 1500, numFiles)
				be.Equal(t, 1501, numDirs)
				be.True(t, sort.SliceIsSorted(entries, func(i, j int) bool {
					return entries[i].Name() < entries[j].Name()
				}))
			},
		}, {
			desc:   "object root",
			bucket: bucket,
			dir:    "root",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket,
					&mock.Object{Key: "root/0=ocfl_object_1.0"},
					&mock.Object{Key: "root/inventory.json"},
					&mock.Object{Key: "root/inventory.json.sha512"},
					&mock.Object{Key: "root/v1/contents/file.txt"},
					&mock.Object{Key: "root/extensions/ext01/config.json"})
			},
			expect: func(t *testing.T, entries []fs.DirEntry, err error) {
				be.NilErr(t, err)
				state := ocfl.ParseObjectDir(entries)
				be.True(t, state.HasNamaste())
				be.True(t, state.HasInventory())
				be.True(t, state.HasSidecar())
				be.True(t, state.HasVersionDir(ocfl.V(1)))
				be.True(t, state.HasExtensions())
				be.Equal(t, 1, len(state.VersionDirs))
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			fsys := s3.NewBucketFS(api, tcase.bucket)
			entries, err := ocflfs.ReadDir(ctx, fsys, tcase.dir)
			tcase.expect(t, entries, err)
		})
	}
}

// TestDirEntriesContract_S3 runs the shared DirEntriesFS contract against the
// S3 backend, using the in-process mock so it runs in CI without a store.
func TestDirEntriesContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	testutil.TestDirEntriesContract(t, fsys)
}
