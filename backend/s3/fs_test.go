package s3_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/s3"
	"github.com/srerickson/ocfl-go/backend/s3/internal/mock"
)

const (
	bucket   = "ocfl-go-test"
	megabyte = 1024 * 1024
	gigabyte = 1024 * megabyte
	mockSeed = 1288108737
	partSize = 6 * megabyte
)

var (
	_ ocfl.FS            = (*s3.BucketFS)(nil)
	_ ocfl.CopyFS        = (*s3.BucketFS)(nil)
	_ ocfl.WriteFS       = (*s3.BucketFS)(nil)
	_ ocfl.ObjectRootsFS = (*s3.BucketFS)(nil)
	_ ocfl.FilesFS       = (*s3.BucketFS)(nil)
)

func TestOpenFile(t *testing.T) {
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
				be.Equal(t, fs.ModeIrregular, info.Mode())
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
			fsys := s3.BucketFS{S3: api, Bucket: tcase.bucket}
			f, err := fsys.OpenFile(ctx, tcase.key)
			tcase.expect(t, f, err)
		})
	}
}

func TestReadDir(t *testing.T) {
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
				state := ocfl.NewObjectRootState(entries)
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
			fsys := s3.BucketFS{S3: api, Bucket: tcase.bucket}
			entries, err := fsys.ReadDir(ctx, tcase.dir)
			tcase.expect(t, entries, err)
		})
	}
}

func TestWrite(t *testing.T) {
	ctx := context.Background()
	bodySize := 201 * megabyte
	body := mock.RandBytes(mockSeed, int64(bodySize))
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
				be.Nonzero(t, state.UpdatedETags["tmp"])

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
				be.Equal(t, expectETag, state.UpdatedETags["tmp"])
				be.Equal(t, bodySize/partSize+1, state.PartCount())
				be.Equal(t, true, state.MPUComplete)
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			fsys := s3.BucketFS{
				S3:                    api,
				Bucket:                tcase.bucket,
				UploadConcurrency:     tcase.uploadConc,
				DefaultUploadPartSize: tcase.uploadPSize,
			}
			val, err := fsys.Write(ctx, tcase.key, tcase.body)
			tcase.expect(t, api, val, err)
		})
	}
}

func TestRemove(t *testing.T) {
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
				be.True(t, state.Deleted["remove-me"])
				be.False(t, state.Deleted["keep-me"])
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			fsys := s3.BucketFS{S3: api, Bucket: tcase.bucket}
			err := fsys.Remove(ctx, tcase.key)
			tcase.expect(t, api, err)
		})
	}
}

func TestRemoveAll(t *testing.T) {
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
				be.True(t, state.Deleted["remove-me/file"])
				be.False(t, state.Deleted["keep-me"])
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			fsys := s3.BucketFS{S3: api, Bucket: tcase.bucket}
			err := fsys.RemoveAll(ctx, tcase.dir)
			tcase.expect(t, api, err)
		})
	}

}

func TestCopy(t *testing.T) {
	ctx := context.Background()
	srcSize := int64(51 * megabyte)
	srcBody := mock.RandBytes(mockSeed, srcSize)
	type testCase struct {
		desc      string
		mock      func(t *testing.T) *mock.S3API
		bucket    string
		copyConc  int
		copyPSize int64
		src       string
		dst       string
		expect    func(*testing.T, *mock.S3API, error)
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
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, state.UpdatedETags["dst-file"])
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
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.NilErr(t, err)
				expETag := mock.ETag(srcBody, partSize)
				be.Equal(t, expETag, state.UpdatedETags["dst-file"])
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			fsys := s3.BucketFS{S3: api, Bucket: tcase.bucket, CopyPartConcurrency: tcase.copyConc, DefaultCopyPartSize: tcase.copyPSize}
			err := fsys.Copy(ctx, tcase.dst, tcase.src)
			tcase.expect(t, api, err)
		})
	}
}

func TestObjectRoots(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		mock   func(t *testing.T) *mock.S3API
		bucket string
		dir    string
		expect func(*testing.T, *mock.S3API, []*ocfl.ObjectRoot, error)
	}
	cases := []testCase{
		{
			desc: "complete object in root",
			dir:  ".",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket,
					&mock.Object{Key: "0=ocfl_object_1.0"},
					&mock.Object{Key: "inventory.json"},
					&mock.Object{Key: "inventory.json.sha512"},
					&mock.Object{Key: "v1/contents/file.txt"},
					&mock.Object{Key: "extensions/ext01/config.json"},
				)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, roots []*ocfl.ObjectRoot, err error) {
				be.NilErr(t, err)
				be.Equal(t, 1, len(roots))
				obj := roots[0]
				be.Nonzero(t, obj.FS)
				be.Equal(t, ".", obj.Path)
				be.Equal(t, "sha512", obj.State.SidecarAlg)
				be.True(t, obj.State.HasInventory())
				be.True(t, obj.State.HasSidecar())
				be.True(t, obj.State.HasNamaste())
				be.True(t, obj.State.HasExtensions())
			},
		}, {
			desc: "complete object in subdir",
			dir:  ".",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket,
					&mock.Object{Key: "a/b/0=ocfl_object_1.0"},
					&mock.Object{Key: "a/b/inventory.json"},
					&mock.Object{Key: "a/b/inventory.json.sha512"},
					&mock.Object{Key: "a/b/v1/contents/file.txt"},
					&mock.Object{Key: "a/b/v2/contents/file2.txt"},
					&mock.Object{Key: "a/b/extensions/ext01/config.json"},
				)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, roots []*ocfl.ObjectRoot, err error) {
				be.NilErr(t, err)
				be.Equal(t, 1, len(roots))
				obj := roots[0]
				be.Nonzero(t, obj.FS)
				be.Equal(t, "a/b", obj.Path)
				be.Equal(t, "sha512", obj.State.SidecarAlg)
				be.Equal(t, 2, len(obj.State.VersionDirs))
				be.True(t, obj.State.HasInventory())
				be.True(t, obj.State.HasSidecar())
				be.True(t, obj.State.HasNamaste())
				be.True(t, obj.State.HasExtensions())
			},
		}, {
			desc: "full storage root",
			dir:  ".",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, mock.StorageRoot(mockSeed, "a-root", 2001)...)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, roots []*ocfl.ObjectRoot, err error) {
				be.NilErr(t, err)
				be.Equal(t, 2001, len(roots))
				for _, obj := range roots {
					be.Nonzero(t, obj.FS)
					be.Equal(t, "sha512", obj.State.SidecarAlg)
					be.True(t, obj.State.HasNamaste())
					be.Equal(t, "1.1", string(obj.State.Spec))
					be.True(t, obj.State.HasInventory())
					be.True(t, obj.State.HasSidecar())
					be.True(t, obj.State.HasNamaste())
					be.True(t, obj.State.HasExtensions())
					be.Equal(t, 1, len(obj.State.VersionDirs))
				}

			},
		}, {
			desc: "ignore duplicate namaste",
			dir:  "a",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket,
					&mock.Object{Key: "a/0=ocfl_object_1.0"},
					&mock.Object{Key: "a/0=ocfl_object_1.1"},
				)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, roots []*ocfl.ObjectRoot, err error) {
				be.NilErr(t, err)
				be.Equal(t, 1, len(roots))
				obj := roots[0]
				be.Equal(t, "1.0", obj.State.Spec)
			},
		}, {
			desc: "ignore nested namaste",
			dir:  ".",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket,
					&mock.Object{Key: "0=ocfl_object_1.0"},
					&mock.Object{Key: "a/0=ocfl_object_1.1"},
				)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, roots []*ocfl.ObjectRoot, err error) {
				be.NilErr(t, err)
				be.Equal(t, 1, len(roots))
				obj := roots[0]
				be.Equal(t, "1.0", obj.State.Spec)
			},
		},
		{
			desc: "invalid path",
			dir:  "../tmp",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, roots []*ocfl.ObjectRoot, err error) {
				isInvalidPathError(t, err)
			},
		},
		{
			desc: "non-conforming before namaste",
			dir:  ".",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket,
					&mock.Object{Key: "00"},
					&mock.Object{Key: "0=ocfl_object_1.0"},
				)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, roots []*ocfl.ObjectRoot, err error) {
				be.NilErr(t, err)
				be.Equal(t, 1, len(roots))
				obj := roots[0]
				be.Equal(t, 1, len(obj.State.NonConform))
			},
		},
		{
			desc: "non-conforming after namaste",
			dir:  ".",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket,
					&mock.Object{Key: "0=ocfl_object_1.0"},
					&mock.Object{Key: "file.txt"},
				)
			},
			bucket: bucket,
			expect: func(t *testing.T, state *mock.S3API, roots []*ocfl.ObjectRoot, err error) {
				be.NilErr(t, err)
				be.Equal(t, 1, len(roots))
				obj := roots[0]
				be.Equal(t, 1, len(obj.State.NonConform))
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			fsys := s3.BucketFS{Bucket: tcase.bucket, S3: api}
			roots := []*ocfl.ObjectRoot{}
			var iterErr error
			fsys.ObjectRoots(ctx, tcase.dir)(func(obj *ocfl.ObjectRoot, err error) bool {
				if err != nil {
					iterErr = err
					return false
				}
				roots = append(roots, obj)
				return true
			})
			tcase.expect(t, api, roots, iterErr)
		})
	}
}
func TestFiles(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		mock   func(t *testing.T) *mock.S3API
		bucket string
		dir    string
		expect func(*testing.T, *mock.S3API, []ocfl.FileInfo, error)
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
			expect: func(t *testing.T, state *mock.S3API, files []ocfl.FileInfo, err error) {
				be.NilErr(t, err)
				be.Equal(t, 5, len(files))
				for _, f := range files {
					be.True(t, strings.HasPrefix(f.Path, "obj/"))
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
			expect: func(t *testing.T, state *mock.S3API, files []ocfl.FileInfo, err error) {
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
			fsys := s3.BucketFS{Bucket: tcase.bucket, S3: api}
			files := []ocfl.FileInfo{}
			var iterErr error
			fsys.Files(ctx, tcase.dir)(func(info ocfl.FileInfo, err error) bool {
				if err != nil {
					iterErr = err
					return false
				}
				files = append(files, info)
				return true
			})
			tcase.expect(t, api, files, iterErr)
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

// func newBackend(t *testing.T) *s3.FS {
// 	ctx := context.Background()
// 	// creds := credentials.NewStaticCredentialsProvider("", "", "")
// 	customResolver := aws.EndpointResolverWithOptionsFunc(
// 		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
// 			return aws.Endpoint{
// 				PartitionID:       "aws",
// 				URL:               "http://localhost:9000",
// 				SigningRegion:     defaultRegion,
// 				HostnameImmutable: true,
// 			}, nil
// 		})
// 	opts := []func(*config.LoadOptions) error{
// 		config.WithDefaultRegion(defaultRegion),
// 		// config.WithCredentialsProvider(creds),
// 		config.WithEndpointResolverWithOptions(customResolver),
// 	}
// 	cfg, err := config.LoadDefaultConfig(ctx, opts...)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	testBucket := randName(testBucketPrefix)
// 	s3client := s3v2.NewFromConfig(cfg)
// 	_, err = s3client.CreateBucket(ctx, &s3v2.CreateBucketInput{
// 		Bucket: aws.String(testBucket),
// 	})
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	t.Log("created test bucket", testBucket)
// 	t.Cleanup(func() {
// 		if err := destroyBucket(ctx, s3client, testBucket); err != nil {
// 			t.Fatal(err)
// 		}
// 		t.Log("removed test bucket", testBucket)
// 	})
// 	return &s3.FS{
// 		S3:     s3client,
// 		Bucket: testBucket,
// 	}
// }

// func destroyBucket(ctx context.Context, s3cl *s3v2.Client, bucket string) error {
// 	b := aws.String(bucket)
// 	listopts := &s3v2.ListObjectsV2Input{Bucket: b}
// 	for {
// 		list, err := s3cl.ListObjectsV2(ctx, listopts)
// 		if err != nil {
// 			return err
// 		}
// 		for _, obj := range list.Contents {
// 			if _, err := s3cl.DeleteObject(ctx, &s3v2.DeleteObjectInput{
// 				Bucket: b,
// 				Key:    obj.Key,
// 			}); err != nil {
// 				return err
// 			}
// 		}
// 		if list.IsTruncated != nil && !*list.IsTruncated {
// 			break
// 		}
// 		listopts.ContinuationToken = list.NextContinuationToken
// 	}
// 	_, err := s3cl.DeleteBucket(ctx, &s3v2.DeleteBucketInput{Bucket: b})
// 	return err
// }

// func randName(prefix string) string {
// 	byt, err := io.ReadAll(io.LimitReader(rand.Reader, 8))
// 	if err != nil {
// 		panic("randName: " + err.Error())
// 	}
// 	return prefix + hex.EncodeToString(byt)
// }
