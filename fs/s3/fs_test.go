package s3_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"

	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

const (
	bucket   = "ocfl-go-test"
	megabyte = 1024 * 1024
	partSize = 6 * megabyte
)

var (
	_ ocflfs.FS           = (*s3.BucketFS)(nil)
	_ ocflfs.DirEntriesFS = (*s3.BucketFS)(nil)
	_ ocflfs.CopyFS       = (*s3.BucketFS)(nil)
	_ ocflfs.WriteFS      = (*s3.BucketFS)(nil)
	_ ocflfs.FileWalker   = (*s3.BucketFS)(nil)
	_ ocflfs.SameBackend  = (*s3.BucketFS)(nil)

	fixtures = filepath.Join("..", "..", "testdata", "content-fixture")
)

func TestOpenFile(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	fixtureFS := ocflfs.DirFS(fixtures)
	fsys := testutil.TmpS3FS(t, fixtureFS)
	type test struct {
		ctx    context.Context
		name   string
		expect func(*testing.T, fs.File, error)
	}
	tests := map[string]test{
		"open file": {
			name: "hello.csv",
			expect: func(t *testing.T, f fs.File, err error) {
				be.NilErr(t, err)
				bytes, err := io.ReadAll(f)
				be.NilErr(t, err)
				be.Equal(t, `1,2,3,"strings"`, string(bytes))
				info, err := f.Stat()
				be.NilErr(t, err)
				fixtureInfo, err := ocflfs.StatFile(context.Background(), fixtureFS, "hello.csv")
				be.NilErr(t, err)
				compareFileInf(t, info, fixtureInfo)
				sys := info.Sys()
				be.Nonzero(t, sys)
				objMeta, isHeadObjectOutput := sys.(*s3v2.HeadObjectOutput)
				be.True(t, isHeadObjectOutput)
				be.Equal(t, *objMeta.ContentLength, info.Size())
				be.Equal(t, *objMeta.LastModified, info.ModTime())
			},
		},
		"open prefix": {
			name: "folder1",
			expect: func(t *testing.T, f fs.File, err error) {
				be.Nonzero(t, err)
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
		"open missing": {
			name: "missing-file.txt",
			expect: func(t *testing.T, f fs.File, err error) {
				be.Nonzero(t, err)
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
	}
	for desc, test := range tests {
		t.Run(desc, func(t *testing.T) {
			ctx := test.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			f, err := fsys.OpenFile(ctx, test.name)
			test.expect(t, f, err)
		})
	}

	t.Run("err if modified", func(t *testing.T) {
		// if a file is modified after opening it, read should fail
		ctx := t.Context()
		key := "file"
		body1 := strings.NewReader("content1")
		body2 := strings.NewReader("content2")
		_, err := fsys.Write(ctx, key, body1) // create
		be.NilErr(t, err)
		f, err := fsys.OpenFile(ctx, key) // open
		be.NilErr(t, err)
		_, err = fsys.Write(ctx, key, body2) // modify
		be.NilErr(t, err)
		_, err = io.ReadAll(f) // read fails with "PreconditionFailed"
		be.Nonzero(t, err)
		var apiErr smithy.APIError
		be.True(t, errors.As(err, &apiErr))
		be.Equal(t, "PreconditionFailed", apiErr.ErrorCode())
	})
}

func TestWriteReadDeleteFile(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	ctx := t.Context()
	fsys := testutil.TmpS3FS(t, nil)
	key := "dir/test-data"
	buff := mock.RandBytes(15 * megabyte)
	n, err := fsys.Write(ctx, key, bytes.NewReader(buff))
	be.NilErr(t, err)
	be.Equal(t, len(buff), int(n))
	for entry, err := range fsys.DirEntries(ctx, "dir") {
		be.NilErr(t, err)
		be.Equal(t, "test-data", entry.Name())
	}
	f, err := fsys.OpenFile(ctx, key)
	be.NilErr(t, err)
	outBytes, err := io.ReadAll(f)
	be.NilErr(t, err)
	be.True(t, bytes.Equal(outBytes, buff))
	be.NilErr(t, fsys.Remove(ctx, key))
}

func TestWriteWithOptions(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	ctx := t.Context()
	fsys := testutil.TmpS3FS(t, nil)
	// option to require key to not exist
	opt := func(input *s3v2.PutObjectInput) {
		match := "*"
		input.IfNoneMatch = &match
	}
	key := "file"
	body := strings.NewReader("content")
	// first write creates the file
	_, err := fsys.WriteWithOptions(ctx, key, body, opt)
	be.NilErr(t, err)

	// second write fails because key exists
	_, err = fsys.WriteWithOptions(ctx, key, body, opt)
	be.Nonzero(t, err)
	var apiErr smithy.APIError
	be.True(t, errors.As(err, &apiErr))
	be.Equal(t, "PreconditionFailed", apiErr.ErrorCode())
}

func TestOpenFile_Mock(t *testing.T) {
	type testCase struct {
		desc   string
		bucket string
		key    string
		mock   func(*testing.T) *mock.S3API
		expect func(*testing.T, fs.File, error)
	}
	ctx := context.Background()
	obj := &mock.Object{
		Key:          "dir/file.tiff",
		Body:         []byte("content"),
		LastModified: time.Now(),
	}
	cases := []testCase{
		{
			desc:   "valid input",
			key:    obj.Key,
			bucket: bucket,
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, obj)
			},
			expect: func(t *testing.T, f fs.File, err error) {
				be.NilErr(t, err)
				body, err := io.ReadAll(f)
				be.NilErr(t, err)
				be.DeepEqual(t, obj.Body, body)
				info, err := f.Stat()
				be.NilErr(t, err)
				be.Equal(t, int64(len(body)), info.Size())
				be.Equal(t, obj.LastModified, info.ModTime())
				be.Equal(t, fs.ModeIrregular|0644, info.Mode())
				be.Equal(t, false, info.IsDir())
				be.Nonzero(t, info.Sys())
			},
		}, {
			desc:   "ErrNotExist",
			key:    "missing",
			bucket: bucket,
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket)
			},
			expect: func(t *testing.T, _ fs.File, err error) {
				isPathError(t, err)
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		}, {
			desc: "invalid path",
			key:  ".",
			expect: func(t *testing.T, _ fs.File, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc: "invalid path",
			key:  "../invalid",
			expect: func(t *testing.T, _ fs.File, err error) {
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
			f, err := fsys.OpenFile(ctx, tcase.key)
			tcase.expect(t, f, err)
		})
	}
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

func TestWrite_Mock(t *testing.T) {
	ctx := context.Background()
	bodySize := 201 * megabyte
	body := mock.RandBytes(int64(bodySize))
	type testCase struct {
		desc        string
		bucket      string
		key         string
		body        io.Reader
		uploadConc  int
		uploadPSize int64
		mock        func(*testing.T) *mock.S3API
		expect      func(*testing.T, *mock.S3API, int64, error)
	}
	cases := []testCase{
		{
			desc: "invalid path",
			key:  "../file.txt",
			expect: func(t *testing.T, _ *mock.S3API, size int64, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc:   "small write",
			bucket: bucket,
			key:    "tmp",
			body:   strings.NewReader("some content"),
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket)
			},
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, state.UpdatedETag("tmp"))

			},
		}, {
			desc:        "multipart",
			bucket:      bucket,
			key:         "tmp",
			uploadPSize: partSize,
			body:        bytes.NewReader(body),
			mock: func(t *testing.T) *mock.S3API {
				api := mock.New(bucket)
				return api
			},
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(bodySize), size)
				expectETag := mock.ETag(body, partSize)
				be.Equal(t, expectETag, state.UpdatedETag("tmp"))
				be.Equal(t, bodySize/partSize+1, state.PartCount())
				be.True(t, state.MPUCompleteFlag())
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			uploaderOpt := func(u *manager.Uploader) {
				u.Concurrency = tcase.uploadConc
				u.PartSize = tcase.uploadPSize
			}
			fsys := s3.NewBucketFS(api, tcase.bucket, s3.WithUploaderOptions(uploaderOpt))
			val, err := fsys.Write(ctx, tcase.key, tcase.body)
			tcase.expect(t, api, val, err)
		})
	}
}

func TestRemove_Mock(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		bucket string
		key    string
		mock   func(*testing.T) *mock.S3API
		expect func(*testing.T, *mock.S3API, error)
	}
	cases := []testCase{
		{
			desc: "invalid path",
			key:  "../file.txt",
			expect: func(t *testing.T, _ *mock.S3API, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc:   "remove file",
			bucket: bucket,
			key:    "remove-me",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{Key: "remove-me"}, &mock.Object{Key: "keep-me"})
			},
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.NilErr(t, err)
				be.True(t, state.WasDeleted("remove-me"))
				be.False(t, state.WasDeleted("keep-me"))
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
			err := fsys.Remove(ctx, tcase.key)
			tcase.expect(t, api, err)
		})
	}
}

func TestRemoveAll_Mock(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		bucket string
		dir    string
		mock   func(*testing.T) *mock.S3API
		expect func(*testing.T, *mock.S3API, error)
	}
	cases := []testCase{
		{
			desc: "invalid path",
			dir:  "..",
			expect: func(t *testing.T, _ *mock.S3API, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc:   "remove dir",
			bucket: bucket,
			dir:    "remove-me",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{Key: "remove-me/file"}, &mock.Object{Key: "keep-me"})
			},
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.NilErr(t, err)
				be.True(t, state.WasDeleted("remove-me/file"))
				be.False(t, state.WasDeleted("keep-me"))
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
			err := fsys.RemoveAll(ctx, tcase.dir)
			tcase.expect(t, api, err)
		})
	}

}

func TestCopy_Mock(t *testing.T) {
	ctx := context.Background()
	srcSize := int64(51 * megabyte)
	srcBody := mock.RandBytes(srcSize)
	type testCase struct {
		desc      string
		mock      func(t *testing.T) *mock.S3API
		bucket    string
		copyConc  int
		copyPSize int64
		src       string
		dst       string
		expect    func(*testing.T, *mock.S3API, int64, error)
	}
	cases := []testCase{
		{
			desc: "simple copy",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{
					Key:  "src-file",
					Body: []byte("some content"),
				})
			},
			bucket: bucket,
			src:    "src-file",
			dst:    "dst-file",
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, state.UpdatedETag("dst-file"))
				be.Nonzero(t, size)
				be.Equal(t, 0, state.PartCount())
			},
		}, {
			desc: "multipart copy",
			mock: func(t *testing.T) *mock.S3API {
				api := mock.New(bucket, &mock.Object{
					Key:  "src-file",
					Body: srcBody,
				})
				// override the default CopyObject method to return
				// the necessary error for initiating multipart copy
				api.CopyObjectFunc = func(_ context.Context, _ *s3v2.CopyObjectInput, _ ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
					return nil, errors.New("copy source is larger than the maximum allowable size")
				}
				return api
			},
			bucket:    bucket,
			src:       "src-file",
			dst:       "dst-file",
			copyPSize: partSize,
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, size)
				expETag := mock.ETag(srcBody, partSize)
				be.Equal(t, expETag, state.UpdatedETag("dst-file"))
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			copyOpts := func(mc *s3.MultiCopier) {
				mc.Concurrency = tcase.copyConc
				mc.PartSize = tcase.copyPSize
			}
			fsys := s3.NewBucketFS(api, tcase.bucket,
				s3.WithMultiPartCopyOption(copyOpts))
			size, err := fsys.Copy(ctx, tcase.dst, tcase.src)
			tcase.expect(t, api, size, err)
		})
	}
}

func TestSameBackend_Mock(t *testing.T) {
	client1 := mock.New(bucket)
	client2 := mock.New(bucket)
	type testCase struct {
		desc   string
		fsys   *s3.BucketFS
		other  ocflfs.FS
		expect bool
	}
	cases := []testCase{
		{
			desc:   "same bucket and client",
			fsys:   s3.NewBucketFS(client1, bucket),
			other:  s3.NewBucketFS(client1, bucket),
			expect: true,
		}, {
			desc:   "different bucket",
			fsys:   s3.NewBucketFS(client1, bucket),
			other:  s3.NewBucketFS(client1, "other-bucket"),
			expect: false,
		}, {
			desc:   "different client",
			fsys:   s3.NewBucketFS(client1, bucket),
			other:  s3.NewBucketFS(client2, bucket),
			expect: false,
		}, {
			desc:   "not a *BucketFS",
			fsys:   s3.NewBucketFS(client1, bucket),
			other:  ocflfs.DirFS(t.TempDir()),
			expect: false,
		}, {
			// The client is caller-supplied and can itself be non-comparable
			// (a struct value carrying a map, say). SameBackend must guard
			// against that rather than reproduce the dstFS == srcFS panic one
			// level down.
			desc:   "non-comparable client",
			fsys:   s3.NewBucketFS(stubClient{m: map[string]string{}}, bucket),
			other:  s3.NewBucketFS(stubClient{m: map[string]string{}}, bucket),
			expect: false,
		},
	}
	for _, tcase := range cases {
		t.Run(tcase.desc, func(t *testing.T) {
			be.Equal(t, tcase.expect, tcase.fsys.SameBackend(tcase.other))
		})
	}
}

func TestCopyDispatch_Mock(t *testing.T) {
	ctx := context.Background()
	api := mock.New(bucket, &mock.Object{Key: "src-file", Body: []byte("some content")})
	// Two separately constructed *BucketFS values over the same bucket and
	// client are the same backend: ocflfs.Copy should take the server-side
	// fast path (one CopyObject), not a GetObject/PutObject round trip.
	dstFS := s3.NewBucketFS(api, bucket)
	srcFS := s3.NewBucketFS(api, bucket)
	_, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
	be.NilErr(t, err)
	be.Equal(t, 1, api.CallCount("CopyObject"))
	be.Equal(t, 0, api.CallCount("GetObject"))
	be.Equal(t, 0, api.CallCount("PutObject"))
}

// stubClient is a minimal s3.S3API implementation whose dynamic type carries
// a map field, so it is not comparable with ==. It's never actually called:
// it exists only to exercise BucketFS.SameBackend's guard against a
// non-comparable client.
type stubClient struct {
	m map[string]string
}

func (stubClient) HeadObject(context.Context, *s3v2.HeadObjectInput, ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	return nil, nil
}

func (stubClient) GetObject(context.Context, *s3v2.GetObjectInput, ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
	return nil, nil
}

func (stubClient) ListObjectsV2(context.Context, *s3v2.ListObjectsV2Input, ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	return nil, nil
}

func (stubClient) PutObject(context.Context, *s3v2.PutObjectInput, ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
	return nil, nil
}

func (stubClient) UploadPart(context.Context, *s3v2.UploadPartInput, ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
	return nil, nil
}

func (stubClient) CreateMultipartUpload(context.Context, *s3v2.CreateMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
	return nil, nil
}

func (stubClient) CompleteMultipartUpload(context.Context, *s3v2.CompleteMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	return nil, nil
}

func (stubClient) AbortMultipartUpload(context.Context, *s3v2.AbortMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	return nil, nil
}

func (stubClient) UploadPartCopy(context.Context, *s3v2.UploadPartCopyInput, ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error) {
	return nil, nil
}

func (stubClient) CopyObject(context.Context, *s3v2.CopyObjectInput, ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
	return nil, nil
}

func (stubClient) DeleteObject(context.Context, *s3v2.DeleteObjectInput, ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	return nil, nil
}

var _ s3.S3API = stubClient{}

func TestWalkFiles_Mock(t *testing.T) {
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

func isInvalidPathError(t *testing.T, err error) {
	t.Helper()
	isPathError(t, err)
	if !errors.Is(err, fs.ErrInvalid) {
		t.Error("error is not fs.ErrInvalid")
	}
}

func isPathError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected non-nil error")
		return
	}
	var pErr *fs.PathError
	if !errors.As(err, &pErr) {
		t.Error("error is not fs.PathError")
	}
}

func compareFileInf(t *testing.T, info, fixture fs.FileInfo) {
	t.Helper()
	be.Equal(t, fixture.Name(), info.Name())
	be.Equal(t, fixture.IsDir(), info.IsDir())
	if !fixture.IsDir() {
		be.Equal(t, fixture.Size(), info.Size())
	}
}

func comparDirEntries(
	t *testing.T,
	entries iter.Seq2[fs.DirEntry, error],
	fixtures iter.Seq2[fs.DirEntry, error],
) {
	t.Helper()
	nextFixture2, stop := iter.Pull2(fixtures)
	defer stop()
	for entry, err := range entries {
		fixtureEntry, fixtureErr, ok := nextFixture2()
		be.True(t, ok)
		be.Equal(t, fixtureErr, err)
		if err != nil {
			be.Zero(t, entry)
			continue
		}
		be.Equal(t, fixtureEntry.Name(), entry.Name())
		be.Equal(t, fixtureEntry.IsDir(), entry.IsDir())
		fixtureInfo, err := fixtureEntry.Info()
		be.NilErr(t, err)
		entryInfo, err := entry.Info()
		be.NilErr(t, err)
		compareFileInf(t, fixtureInfo, entryInfo)
	}
	// no more fixture entries
	_, _, more := nextFixture2()
	be.False(t, more)
}

func TestSeek_Mock(t *testing.T) {
	ctx := context.Background()
	content := []byte("Hello, World! This is test content for seeking.")
	obj := &mock.Object{
		Key:          "seekable-file.txt",
		Body:         content,
		LastModified: time.Now(),
	}

	type testCase struct {
		desc   string
		offset int64
		whence int
		expect func(*testing.T, fs.File, int64, error)
	}

	cases := []testCase{
		{
			desc:   "SeekStart to beginning",
			offset: 0,
			whence: io.SeekStart,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(0), pos)
				buf := make([]byte, 5)
				n, err := f.Read(buf)
				be.NilErr(t, err)
				be.Equal(t, 5, n)
				be.Equal(t, "Hello", string(buf))
			},
		},
		{
			desc:   "SeekStart to middle",
			offset: 7,
			whence: io.SeekStart,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(7), pos)
				buf := make([]byte, 5)
				n, err := f.Read(buf)
				be.NilErr(t, err)
				be.Equal(t, 5, n)
				be.Equal(t, "World", string(buf))
			},
		},
		{
			desc:   "SeekEnd negative offset",
			offset: -8,
			whence: io.SeekEnd,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(len(content)-8), pos)
				buf := make([]byte, 8)
				n, err := f.Read(buf)
				be.NilErr(t, err)
				be.Equal(t, 8, n)
				be.Equal(t, "seeking.", string(buf))
			},
		},
		{
			desc:   "SeekStart negative offset error",
			offset: -1,
			whence: io.SeekStart,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.Nonzero(t, err)
			},
		},
		{
			desc:   "SeekStart past EOF",
			offset: int64(len(content) + 10),
			whence: io.SeekStart,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(len(content)+10), pos)
				// Read should return EOF
				buf := make([]byte, 5)
				_, readErr := f.Read(buf)
				be.True(t, errors.Is(readErr, io.EOF))
			},
		},
	}

	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			api := mock.New(bucket, obj)
			fsys := s3.NewBucketFS(api, bucket)
			f, err := fsys.OpenFile(ctx, obj.Key)
			be.NilErr(t, err)
			defer f.Close()

			seeker, ok := f.(io.Seeker)
			be.True(t, ok)

			pos, err := seeker.Seek(tcase.offset, tcase.whence)
			tcase.expect(t, f, pos, err)
		})
	}
}

func TestSeekCurrent_Mock(t *testing.T) {
	ctx := context.Background()
	content := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	obj := &mock.Object{
		Key:          "alphabet.txt",
		Body:         content,
		LastModified: time.Now(),
	}

	api := mock.New(bucket, obj)
	fsys := s3.NewBucketFS(api, bucket)
	f, err := fsys.OpenFile(ctx, obj.Key)
	be.NilErr(t, err)
	defer f.Close()

	seeker := f.(io.Seeker)

	// Read first 5 bytes
	buf := make([]byte, 5)
	n, err := f.Read(buf)
	be.NilErr(t, err)
	be.Equal(t, 5, n)
	be.Equal(t, "ABCDE", string(buf))

	// Seek forward 5 from current (should be at position 10)
	pos, err := seeker.Seek(5, io.SeekCurrent)
	be.NilErr(t, err)
	be.Equal(t, int64(10), pos)

	// Read next 5 bytes
	n, err = f.Read(buf)
	be.NilErr(t, err)
	be.Equal(t, 5, n)
	be.Equal(t, "KLMNO", string(buf))

	// Seek backward from current
	pos, err = seeker.Seek(-10, io.SeekCurrent)
	be.NilErr(t, err)
	be.Equal(t, int64(5), pos)

	// Read next 5 bytes
	n, err = f.Read(buf)
	be.NilErr(t, err)
	be.Equal(t, 5, n)
	be.Equal(t, "FGHIJ", string(buf))
}

func TestSeekWithZip(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running")
	}
	ctx := context.Background()

	// Create a zip file in memory
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	files := map[string]string{
		"hello.txt":      "Hello, World!",
		"dir/nested.txt": "Nested content",
		"numbers.txt":    "1234567890",
	}
	for name, content := range files {
		fw, err := zw.Create(name)
		be.NilErr(t, err)
		_, err = fw.Write([]byte(content))
		be.NilErr(t, err)
	}
	be.NilErr(t, zw.Close())

	// Upload zip to real S3
	fsys := testutil.TmpS3FS(t, nil)
	_, err := fsys.Write(ctx, "archive.zip", &zipBuf)
	be.NilErr(t, err)

	// Open the zip file from S3
	f, err := fsys.OpenFile(ctx, "archive.zip")
	be.NilErr(t, err)
	defer f.Close()

	// Get file info for size
	info, err := f.Stat()
	be.NilErr(t, err)

	// Create a ReaderAt from our ReadSeeker
	seeker := f.(io.ReadSeeker)
	readerAt := &seekerReaderAt{rs: seeker}

	// Open as zip archive - this requires seeking!
	zr, err := zip.NewReader(readerAt, info.Size())
	be.NilErr(t, err)

	// Verify we can read all files
	be.Equal(t, len(files), len(zr.File))

	for _, zf := range zr.File {
		expectedContent, exists := files[zf.Name]
		be.True(t, exists)

		rc, err := zf.Open()
		be.NilErr(t, err)

		content, err := io.ReadAll(rc)
		be.NilErr(t, err)
		rc.Close()

		be.Equal(t, expectedContent, string(content))
	}
}

// seekerReaderAt adapts an io.ReadSeeker to io.ReaderAt
type seekerReaderAt struct {
	rs io.ReadSeeker
}

func (s *seekerReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	_, err = s.rs.Seek(off, io.SeekStart)
	if err != nil {
		return 0, err
	}
	return s.rs.Read(p)
}

// failDeleteAPI is the mock with DeleteObject made to fail for one chosen key.
// The mock records calls but injects no errors, and does not need to: the
// method set it satisfies is an interface, so shadowing one method on an
// embedding type is enough to stand in for the whole thing.
type failDeleteAPI struct {
	*mock.S3API
	failKey string
	err     error
	// attempted records every key DeleteObject was called with. The mock's
	// own call log cannot serve here: the failing key never reaches it.
	attempted []string
}

func (a *failDeleteAPI) DeleteObject(ctx context.Context, in *s3v2.DeleteObjectInput,
	opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	if in.Key != nil {
		a.attempted = append(a.attempted, *in.Key)
	}
	if in.Key != nil && *in.Key == a.failKey {
		return nil, a.err
	}
	return a.S3API.DeleteObject(ctx, in, opts...)
}

// TestRemoveAllBestEffort pins the WriteFS.RemoveAll contract that one key
// which will not delete must not abandon the keys after it. The subtree case
// is the one this changed: removeAll used to return on the first failing
// DeleteObject, leaving every later key in place and reporting only that one
// failure.
func TestRemoveAllBestEffort(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	// Keys are chosen so the failing one sorts in the middle: a listing is
	// returned in key order, so a survivor after it is what proves the loop
	// kept going.
	for _, tc := range []struct {
		name    string
		remove  string
		keys    []string
		failKey string
	}{
		{
			name:    "dot",
			remove:  ".",
			keys:    []string{"a.txt", "b.txt", "c.txt"},
			failKey: "b.txt",
		},
		{
			name:    "subtree",
			remove:  "dir",
			keys:    []string{"dir/a.txt", "dir/b.txt", "dir/c.txt"},
			failKey: "dir/b.txt",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := make([]*mock.Object, 0, len(tc.keys))
			for _, key := range tc.keys {
				objects = append(objects, &mock.Object{Key: key, Body: []byte(key)})
			}
			api := &failDeleteAPI{
				S3API:   mock.New(bucket, objects...),
				failKey: tc.failKey,
				err:     boom,
			}
			fsys := s3.NewBucketFS(api, bucket)

			err := fsys.RemoveAll(ctx, tc.remove)
			be.True(t, err != nil)
			be.True(t, errors.Is(err, boom))

			// Every key was attempted, the failing one included.
			attempted := slices.Clone(api.attempted)
			sort.Strings(attempted)
			be.AllEqual(t, tc.keys, attempted)

			// And every key but the failing one is actually gone.
			for _, key := range tc.keys {
				if key == tc.failKey {
					be.False(t, api.WasDeleted(key))
					continue
				}
				if !api.WasDeleted(key) {
					t.Errorf("%q was not deleted; RemoveAll(%q) abandoned it", key, tc.remove)
				}
			}
		})
	}
}

// headErrAPI is the mock with HeadObject made to fail with a chosen error, so
// a test can present an error shape the mock does not itself produce.
type headErrAPI struct {
	*mock.S3API
	err error
}

var _ s3.S3API = (*headErrAPI)(nil)

func (a *headErrAPI) HeadObject(ctx context.Context, in *s3v2.HeadObjectInput,
	opts ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	return nil, a.err
}

// respErr builds the error an endpoint's 404 arrives as: the API error the
// SDK deserialized, inside the transport error carrying the response.
func respErr(status int, inner error) error {
	return &awshttp.ResponseError{
		ResponseError: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{
				Response: &http.Response{StatusCode: status, Header: http.Header{}},
			},
			Err: inner,
		},
		RequestID: "TESTREQUESTID",
	}
}

// TestNotExistMapping_Mock covers which S3 API errors mean "not there" and
// what the backend does with the ones that do.
//
// The shapes matter because they are not interchangeable: HeadObject has no
// response body, so a real endpoint's 404 deserializes to *types.NotFound
// rather than the *types.NoSuchKey a GetObject body carries, and an
// S3-compatible store the SDK cannot type at all yields a
// *smithy.GenericAPIError holding the code as a string. A check written
// against one shape passes CI against a mock that speaks it and fails against
// a store that does not.
func TestNotExistMapping_Mock(t *testing.T) {
	ctx := context.Background()

	t.Run("recognized shapes", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
		}{
			{"typed NotFound", respErr(404, &types.NotFound{})},
			{"typed NoSuchKey", respErr(404, &types.NoSuchKey{})},
			{"generic code NotFound", respErr(404, &smithy.GenericAPIError{Code: "NotFound"})},
			{"generic code NoSuchKey", respErr(404, &smithy.GenericAPIError{Code: "NoSuchKey"})},
			// A store whose code neither the SDK nor this package knows:
			// the 404 is the only thing left to go on, and it is enough.
			{"unrecognized code with 404", respErr(404, &smithy.GenericAPIError{Code: "KeyNotPresent"})},
			// Bare typed errors, with no transport error around them.
			{"bare NotFound", &types.NotFound{}},
			{"bare NoSuchKey", &types.NoSuchKey{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fsys := s3.NewBucketFS(&headErrAPI{S3API: mock.New(bucket), err: tc.err}, bucket)
				_, err := fsys.OpenFile(ctx, "missing.txt")
				be.True(t, errors.Is(err, fs.ErrNotExist))
			})
		}
	})

	t.Run("not-exist shapes it must not claim", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
		}{
			// A missing bucket is a 404 too, and is a configuration error
			// rather than a missing file. Reporting fs.ErrNotExist would
			// tell a caller the object was never written when in fact
			// nothing can be read or written at all.
			{"typed NoSuchBucket", respErr(404, &types.NoSuchBucket{})},
			{"generic code NoSuchBucket", respErr(404, &smithy.GenericAPIError{Code: "NoSuchBucket"})},
			{"access denied", respErr(403, &smithy.GenericAPIError{Code: "AccessDenied"})},
			{"server error", respErr(500, &smithy.GenericAPIError{Code: "InternalError"})},
			{"transport failure", errors.New("dial tcp: connection refused")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fsys := s3.NewBucketFS(&headErrAPI{S3API: mock.New(bucket), err: tc.err}, bucket)
				_, err := fsys.OpenFile(ctx, "missing.txt")
				be.True(t, err != nil)
				be.False(t, errors.Is(err, fs.ErrNotExist))
			})
		}
	})

	// The mapping wraps fs.ErrNotExist around the cause instead of replacing
	// it. Replacing threw away the status code, the request ID and the API
	// error code -- most of what makes a failure against a real endpoint
	// diagnosable -- and left a caller with an error it could match but not
	// read.
	t.Run("the cause survives the wrap", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			open func() error
		}{
			{
				name: "OpenFile",
				open: func() error {
					fsys := s3.NewBucketFS(mock.New(bucket), bucket)
					_, err := fsys.OpenFile(ctx, "missing.txt")
					return err
				},
			},
			{
				name: "Copy",
				open: func() error {
					fsys := s3.NewBucketFS(mock.New(bucket), bucket)
					_, err := fsys.Copy(ctx, "dst.txt", "missing.txt")
					return err
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.open()
				be.True(t, errors.Is(err, fs.ErrNotExist))

				// Still a *fs.PathError naming the file, as before.
				var pathErr *fs.PathError
				be.True(t, errors.As(err, &pathErr))
				be.Equal(t, "missing.txt", pathErr.Path)

				// And the API error underneath is still reachable.
				var apiErr smithy.APIError
				be.True(t, errors.As(err, &apiErr))
				be.Equal(t, "NotFound", apiErr.ErrorCode())
				var respErr *smithyhttp.ResponseError
				be.True(t, errors.As(err, &respErr))
				be.Equal(t, 404, respErr.HTTPStatusCode())
				var reqIDErr interface{ ServiceRequestID() string }
				be.True(t, errors.As(err, &reqIDErr))
				be.Equal(t, mock.RequestID(), reqIDErr.ServiceRequestID())
			})
		}
	})

	// The generic style is the one the old typed-only check missed: a store
	// whose error the SDK cannot resolve to *types.NotFound or
	// *types.NoSuchKey still has to read as a missing file end to end.
	t.Run("generic style backend", func(t *testing.T) {
		api := mock.New(bucket, &mock.Object{Key: "present.txt", Body: []byte("x")}).
			WithNotFoundStyle(mock.NotFoundStyleGeneric)
		fsys := s3.NewBucketFS(api, bucket)

		_, err := fsys.OpenFile(ctx, "missing.txt")
		be.True(t, errors.Is(err, fs.ErrNotExist))

		_, err = fsys.Copy(ctx, "dst.txt", "missing.txt")
		be.True(t, errors.Is(err, fs.ErrNotExist))

		// A key that is there still opens: the mapping did not turn every
		// error into a missing file.
		file, err := fsys.OpenFile(ctx, "present.txt")
		be.NilErr(t, err)
		be.NilErr(t, file.Close())
	})
}
