package s3_test

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

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
