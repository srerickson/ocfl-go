package s3_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// listErrAPI embeds the standard mock.S3API and fails ListObjectsV2 for a
// chosen prefix, so the shared WalkFiles contract can observe how BucketFS
// delivers a walk failure: the error must reach the caller wrapped in an
// *fs.PathError and the underlying API error must stay reachable through
// the chain.
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

// TestWalkFilesContract_S3 runs the shared WalkFiles contract tests
// (internal/testutil) against the S3 backend. It exercises the exact
// BucketFS.WalkFiles method the library uses (ocflfs.WalkFiles dispatches to
// it because BucketFS implements FileWalker): the wrapper that stamps the
// backend instance onto every yielded FileRef and stops iterating as soon
// as the callback returns false.
//
// The error fixture is a BucketFS over an API whose ListObjectsV2 fails for
// the "blocked/" prefix, so the shared error-propagation test can walk the
// same name "blocked" the local fixture uses.
func TestWalkFilesContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	testutil.TestWalkFilesContract(t, fsys, testutil.WalkFilesContract{
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

// TestWalkFiles_ListErrorShape pins the S3 backend's error-delivery shape,
// the half of the error-propagation contract the shared suite cannot assert
// (it doesn't know the backend's sentinel): a ListObjectsV2 failure reaches
// the caller as an *fs.PathError with Op "list_files" naming the walked
// directory, and the underlying API error remains reachable through
// errors.Is — never replaced or swallowed.
func TestWalkFiles_ListErrorShape(t *testing.T) {
	ctx := context.Background()
	listErr := errors.New("list failed")
	api := &listErrAPI{
		S3API:       mock.New(bucket, &mock.Object{Key: "ok.txt", Body: []byte("x")}),
		errOnPrefix: "blocked/",
		err:         listErr,
	}
	fsys := s3.NewBucketFS(api, bucket)

	var gotErr error
	var refs []*ocflfs.FileRef
	for ref, err := range fsys.WalkFiles(ctx, "blocked") {
		if err != nil {
			gotErr = err
			break
		}
		if ref != nil {
			refs = append(refs, ref)
		}
	}
	be.Nonzero(t, gotErr)
	be.Equal(t, 0, len(refs))
	var pathErr *fs.PathError
	be.True(t, errors.As(gotErr, &pathErr))
	be.Equal(t, "list_files", pathErr.Op)
	be.Equal(t, "blocked", pathErr.Path)
	// Wrap, don't replace: the exact API failure must remain reachable.
	be.True(t, errors.Is(gotErr, listErr))
	be.True(t, pathErr.Err != fs.ErrNotExist)
}
