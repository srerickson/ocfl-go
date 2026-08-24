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
//   - dir="." on an empty bucket takes the same no-content path (no Prefix is
//     set, fs/s3/s3.go:90-92) and yields fs.ErrNotExist with Path=".".
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

func TestDirEntries_RootEmptyBucket_ErrNotExist(t *testing.T) {
	// Case (3): dir="." on a bucket with no objects at all. No Prefix is set
	// (fs/s3/s3.go:90-92) and the first page is empty, so dirEntries yields
	// fs.ErrNotExist with Path="." -- same as a missing prefix. Note this
	// differs from the local backend, where reading an empty directory
	// yields zero entries and nil error.
	api := mock.New(bucket)
	fsys := s3.NewBucketFS(api, bucket)
	ctx := context.Background()

	entries, err := ocflfs.ReadDir(ctx, fsys, ".")
	be.Zero(t, entries)
	be.Nonzero(t, err)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "readdir", pathErr.Op)
	be.Equal(t, ".", pathErr.Path)
	be.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestDirEntries_RootNonEmptyBucket(t *testing.T) {
	// Contrast for case (3): the ErrNotExist above applies only to a *truly
	// empty* bucket. Once any key exists, "." lists normally, mixing file
	// and common-prefix (subdirectory) entries sorted by name.
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
