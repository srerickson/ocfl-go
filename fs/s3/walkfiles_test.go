package s3_test

// Tests for walkfiles.go: BucketFS.WalkFiles, including the shared
// cross-backend WalkFiles contract.

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestWalkFiles(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		mock   func(t *testing.T) *mock.S3API
		bucket string
		dir    string
		expect func(*testing.T, *mock.S3API, []*ocflfs.FileRef, error)
	}
	cases := []testCase{
		{
			desc: "object in root",
			dir:  "obj",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket,
					&mock.Object{Key: "obj/0=ocfl_object_1.0"},
					&mock.Object{Key: "obj/inventory.json"},
					&mock.Object{Key: "obj/inventory.json.sha512"},
					&mock.Object{Key: "obj/v1/contents/file.txt"},
					&mock.Object{Key: "obj/extensions/ext01/config.json"},
				)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, files []*ocflfs.FileRef, err error) {
				be.NilErr(t, err)
				be.Equal(t, 5, len(files))
				for _, f := range files {
					be.Nonzero(t, f.Info)
					be.True(t, strings.HasPrefix(f.FullPath(), "obj/"))
				}
			},
		},
		{
			desc: "invalid path error",
			dir:  "../tmp",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, files []*ocflfs.FileRef, err error) {
				isInvalidPathError(t, err)
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
			var walkFiles []*ocflfs.FileRef
			var walkErr error
			for f, err := range fsys.WalkFiles(ctx, tcase.dir) {
				if err != nil {
					walkErr = err
					break
				}
				if f != nil {
					walkFiles = append(walkFiles, f)
				}
			}
			tcase.expect(t, api, walkFiles, walkErr)
		})
	}
}

func TestWalkFiles_SkipDirPlaceholders(t *testing.T) {
	// S3 directory placeholder objects (zero-byte keys ending in "/"),
	// which the S3 console and some clients create to represent
	// directories, must not be yielded as files.
	ctx := context.Background()
	api := mock.New(bucket,
		&mock.Object{Key: "dir/"},
		&mock.Object{Key: "dir/file.txt"},
	)
	fsys := s3.NewBucketFS(api, bucket)
	var files []*ocflfs.FileRef
	for f, err := range fsys.WalkFiles(ctx, "dir") {
		be.NilErr(t, err)
		if f != nil {
			files = append(files, f)
		}
	}
	be.Equal(t, 1, len(files))
	be.Equal(t, "file.txt", files[0].Path)
	be.Equal(t, "dir/file.txt", files[0].FullPath())
}

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

// The real mock always emits non-nil Key, Size and LastModified in
// ListObjectsV2 responses, so it cannot represent the partial entries a
// non-AWS FilesAPI implementation might return. stubWalkAPI embeds it and
// lets each test control the exact listing page.
type stubWalkAPI struct {
	*mock.S3API
	listFn func(context.Context, *s3v2.ListObjectsV2Input, ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error)
}

func (s *stubWalkAPI) ListObjectsV2(ctx context.Context, in *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	return s.listFn(ctx, in, opts...)
}

func walkAll(t *testing.T, fsys *s3.BucketFS, dir string) ([]*ocflfs.FileRef, error) {
	t.Helper()
	var files []*ocflfs.FileRef
	var walkErr error
	for f, err := range fsys.WalkFiles(context.Background(), dir) {
		if err != nil {
			walkErr = err
			break
		}
		if f != nil {
			files = append(files, f)
		}
	}
	return files, walkErr
}

func TestWalkFiles_NilKeySkipped(t *testing.T) {
	// A partial listing entry with a nil Key must be skipped rather than
	// dereferenced: dereferencing would panic inside the iterator instead
	// of returning an error. Valid entries in the same page (including
	// directory placeholders) are unaffected.
	now := time.Unix(1700000000, 0).UTC()
	api := &stubWalkAPI{
		S3API: mock.New(bucket),
		listFn: func(_ context.Context, _ *s3v2.ListObjectsV2Input, _ ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
			return &s3v2.ListObjectsV2Output{Contents: []types.Object{
				{Key: nil, Size: aws.Int64(10), LastModified: aws.Time(now)},               // partial: nil Key
				{Key: aws.String("obj/"), Size: aws.Int64(0), LastModified: aws.Time(now)}, // dir placeholder
				{Key: aws.String("obj/file.txt"), Size: aws.Int64(42), LastModified: aws.Time(now)},
			}}, nil
		},
	}
	fsys := s3.NewBucketFS(api, bucket)
	files, err := walkAll(t, fsys, "obj")
	be.NilErr(t, err)
	be.Equal(t, 1, len(files))
	be.Equal(t, "file.txt", files[0].Path)
	be.Equal(t, "obj/file.txt", files[0].FullPath())
}

func TestWalkFiles_NilSizeAndModTime(t *testing.T) {
	// Entries with nil Size and nil LastModified must not panic: both are
	// converted to zero values (aws.ToInt64 / aws.ToTime semantics).
	api := &stubWalkAPI{
		S3API: mock.New(bucket),
		listFn: func(_ context.Context, _ *s3v2.ListObjectsV2Input, _ ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
			return &s3v2.ListObjectsV2Output{Contents: []types.Object{
				{Key: aws.String("obj/partial"), Size: nil, LastModified: nil},
			}}, nil
		},
	}
	fsys := s3.NewBucketFS(api, bucket)
	files, err := walkAll(t, fsys, "obj")
	be.NilErr(t, err)
	be.Equal(t, 1, len(files))
	be.Equal(t, "partial", files[0].Info.Name())
	be.Equal(t, int64(0), files[0].Info.Size())
	be.True(t, files[0].Info.ModTime().IsZero())
}

func TestWalkFiles_NormalEntryUnchanged(t *testing.T) {
	// A fully-populated entry must produce the same FileRef as before the
	// nil guards: path, name, size, mode and modification time preserved.
	now := time.Unix(1700000000, 0).UTC()
	api := &stubWalkAPI{
		S3API: mock.New(bucket),
		listFn: func(_ context.Context, _ *s3v2.ListObjectsV2Input, _ ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
			return &s3v2.ListObjectsV2Output{Contents: []types.Object{
				{Key: aws.String("obj/file.txt"), Size: aws.Int64(1234), LastModified: aws.Time(now)},
			}}, nil
		},
	}
	fsys := s3.NewBucketFS(api, bucket)
	files, err := walkAll(t, fsys, "obj")
	be.NilErr(t, err)
	be.Equal(t, 1, len(files))
	ref := files[0]
	be.Equal(t, "obj", ref.BaseDir)
	be.Equal(t, "file.txt", ref.Path)
	be.Equal(t, "obj/file.txt", ref.FullPath())
	be.Equal(t, "file.txt", ref.Info.Name())
	be.Equal(t, int64(1234), ref.Info.Size())
	be.True(t, ref.Info.ModTime().Equal(now))
	be.Equal(t, fs.ModeIrregular|0644, ref.Info.Mode())
}
