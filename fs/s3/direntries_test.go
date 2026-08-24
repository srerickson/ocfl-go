package s3_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// These tests pin the exact behavior of s3.dirEntries() (fs/s3/s3.go:79) for
// prefixes that have no objects:
//
//   - A prefix with no objects and no common prefixes is treated as a missing
//     directory: dirEntries yields fs.ErrNotExist (fs/s3/s3.go:103-108, the
//     `prefixHasContent` guard introduced at fs/s3/s3.go:93).
//   - A prefix that exists only as a common prefix (object keys are nested
//     one level deeper) is NOT empty: the CommonPrefixes alone count as
//     content (fs/s3/s3.go:100-102,110) and are yielded as subdirectory
//     entries (fs/s3/s3.go:112-117).
//   - dir="." on an empty bucket is the one exception to the missing-directory
//     rule: no Prefix is set (fs/s3/s3.go:90-92) and the first page is empty,
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
	// (fs/s3/s3.go:90-92) and the first page is empty, so dirEntries yields
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
