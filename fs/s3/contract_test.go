package s3_test

import (
	"context"
	"errors"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/internal"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

// The contract callers live in package s3_test, not package s3: the shared
// suite is free to import backend packages, so a backend may not import it
// back.
//
// Each caller builds its BucketFS over the in-process mock, so the whole
// suite runs in CI without an S3 store.

// TestWriteFSWriteContract_S3 runs the shared WriteFS.Write contract against
// the S3 backend.
func TestWriteFSWriteContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	internal.TestWriteFSWriteContract(t, fsys, internal.WriteFSWriteContract{
		WriteDotIsError: true,
	})
}

// TestWriteFSRemoveContract_S3 runs the shared WriteFS.Remove contract against
// the S3 backend. RemoveDotIsNotExist is true: the backend guards "." with
// fs.ErrNotExist, unlike the local backend's descriptive *fs.PathError.
func TestWriteFSRemoveContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	internal.TestWriteFSRemoveContract(t, fsys, internal.WriteFSRemoveContract{
		RemoveDotIsNotExist: true,
		// remove() calls DeleteObject with no existence check, and S3's
		// DeleteObject is idempotent: a key that was never there deletes
		// successfully, so Remove reports nil where the local backend
		// reports fs.ErrNotExist.
		SkipMissingIsNotExist: "Remove of a missing key returns nil on the s3 backend; see #166",
	})
}

// TestWriteFSRemoveAllContract_S3 runs the shared WriteFS.RemoveAll contract
// against the S3 backend. S3 has no directories: "." empties the bucket, and
// a name is a key prefix, so RemoveAll on a file's own path matches nothing.
func TestWriteFSRemoveAllContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	internal.TestWriteFSRemoveAllContract(t, fsys, internal.WriteFSRemoveAllContract{
		RemoveAllDotIsError:      false,
		RemoveAllOnFileRemovesIt: false,
	})
}

// TestDirEntriesContract_S3 runs the shared DirEntriesFS contract against the
// S3 backend.
func TestDirEntriesContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	internal.TestDirEntriesContract(t, fsys)
}

// TestWalkFilesContract_S3 runs the shared WalkFiles contract against the S3
// backend. It exercises the exact BucketFS.WalkFiles method the library uses
// (ocflfs.WalkFiles dispatches to it because BucketFS implements FileWalker):
// the wrapper that stamps the backend instance onto every yielded FileRef and
// stops iterating as soon as the callback returns false.
//
// The error fixture is a BucketFS over an API whose ListObjectsV2 fails for
// the "blocked/" prefix, so the shared error-propagation test can walk the
// same name "blocked" the local fixture uses.
func TestWalkFilesContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	internal.TestWalkFilesContract(t, fsys, internal.WalkFilesContract{
		ErrWalk: func(t *testing.T) ocflfs.WriteFS {
			api := &listErrAPI{
				S3API:       mock.New(bucket),
				errOnPrefix: "blocked/",
				err:         errors.New("list failed"),
			}
			return s3.NewBucketFS(api, bucket)
		},
	})
}

// listErrAPI embeds the standard mock and fails ListObjectsV2 for a chosen
// prefix, so a test can observe how BucketFS delivers a walk failure.
type listErrAPI struct {
	*mock.S3API
	errOnPrefix string
	err         error
}

var _ s3.S3API = (*listErrAPI)(nil)

func (a *listErrAPI) ListObjectsV2(ctx context.Context, in *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	if in.Prefix != nil && *in.Prefix == a.errOnPrefix {
		return nil, a.err
	}
	return a.S3API.ListObjectsV2(ctx, in, opts...)
}
