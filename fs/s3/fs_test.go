package s3_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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

// TestCopySpecialCharacters is the mock cases' counterpart against a real
// endpoint: it proves the copy-source header is encoded correctly for keys
// a bug in url.QueryEscape would corrupt or misroute -- a space, a "+",
// non-ASCII characters, and a nested path -- since the mock alone cannot
// prove what a real bucket does with the header.
func TestCopySpecialCharacters(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	ctx := t.Context()
	fsys := testutil.TmpS3FS(t, nil)
	cases := []struct {
		desc string
		src  string
		dst  string
	}{
		{desc: "space in nested key", src: "dir/sub dir/a file.txt", dst: "dst-dir/a file.txt"},
		{desc: "plus in key", src: "a+b.txt", dst: "dst+c.txt"},
		{desc: "non-ASCII key", src: "café/日本語.txt", dst: "dst-café.txt"},
	}
	for _, tcase := range cases {
		t.Run(tcase.desc, func(t *testing.T) {
			buff := mock.RandBytes(1024)
			_, err := fsys.Write(ctx, tcase.src, bytes.NewReader(buff))
			be.NilErr(t, err)
			size, err := fsys.Copy(ctx, tcase.dst, tcase.src)
			be.NilErr(t, err)
			be.Equal(t, len(buff), int(size))
			f, err := fsys.OpenFile(ctx, tcase.dst)
			be.NilErr(t, err)
			defer f.Close()
			got, err := io.ReadAll(f)
			be.NilErr(t, err)
			be.True(t, bytes.Equal(buff, got))
		})
	}
}

// TestWritePartialReader is the mock cases' counterpart against a real
// endpoint, where a wrongly declared Content-Length fails before the request
// leaves the client.
func TestWritePartialReader(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	ctx := t.Context()
	fsys := testutil.TmpS3FS(t, nil)
	buff := mock.RandBytes(4096)
	const offset = 1000

	r := bytes.NewReader(buff)
	_, err := r.Seek(offset, io.SeekStart)
	be.NilErr(t, err)

	key := "partial"
	n, err := fsys.Write(ctx, key, r)
	be.NilErr(t, err)
	be.Equal(t, int64(len(buff)-offset), n)

	f, err := fsys.OpenFile(ctx, key)
	be.NilErr(t, err)
	defer f.Close()
	got, err := io.ReadAll(f)
	be.NilErr(t, err)
	be.True(t, bytes.Equal(buff[offset:], got))
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
		// expectContent, when set, is read back through OpenFile and compared
		// to what the bucket now holds under key. The mock materializes
		// objects, so this asserts the bytes that landed, not just the
		// requests that were sent.
		expectContent *string
	}
	content := func(s string) *string { return &s }
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
		}, {
			// Write reads r from where it is, not from where it started.
			desc:   "partially consumed reader",
			bucket: bucket,
			key:    "tmp",
			body: func() io.Reader {
				r := bytes.NewReader([]byte("0123456789"))
				if _, err := r.Seek(3, io.SeekStart); err != nil {
					panic(err)
				}
				return r
			}(),
			expectContent: content("3456789"),
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket)
			},
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(7), size)
			},
		}, {
			// Nothing left to read is an empty object, not a failure.
			desc:   "exhausted reader",
			bucket: bucket,
			key:    "tmp",
			body: func() io.Reader {
				r := bytes.NewReader([]byte("already read"))
				if _, err := io.ReadAll(r); err != nil {
					panic(err)
				}
				return r
			}(),
			expectContent: content(""),
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket)
			},
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(0), size)
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
			if tcase.expectContent != nil {
				f, err := fsys.OpenFile(ctx, tcase.key)
				be.NilErr(t, err)
				defer f.Close()
				got, err := io.ReadAll(f)
				be.NilErr(t, err)
				be.Equal(t, *tcase.expectContent, string(got))
			}
		})
	}
}

// TestWriteContentLengthOption_Mock pins what Write does with the request's
// ContentLength: it sets none of its own, and forwards a caller's untouched,
// including when it is wrong.
func TestWriteContentLengthOption_Mock(t *testing.T) {
	ctx := context.Background()
	const key, body = "tmp", "some content"

	withLength := func(n int64) func(*s3v2.PutObjectInput) {
		return func(in *s3v2.PutObjectInput) { in.ContentLength = &n }
	}

	t.Run("write declares no length of its own", func(t *testing.T) {
		// A reader whose total size and remaining bytes differ, so a declared
		// length taken from either would be visible to the mock.
		r := bytes.NewReader([]byte(body))
		_, err := r.Seek(5, io.SeekStart)
		be.NilErr(t, err)
		api := mock.New(bucket)
		fsys := s3.NewBucketFS(api, bucket)
		n, err := fsys.Write(ctx, key, r)
		be.NilErr(t, err)
		be.Equal(t, int64(len(body)-5), n)
	})

	t.Run("matching option is honored", func(t *testing.T) {
		api := mock.New(bucket)
		fsys := s3.NewBucketFS(api, bucket)
		n, err := fsys.WriteWithOptions(ctx, key, strings.NewReader(body), withLength(int64(len(body))))
		be.NilErr(t, err)
		be.Equal(t, int64(len(body)), n)
	})

	t.Run("wrong option is passed through, not corrected", func(t *testing.T) {
		api := mock.New(bucket)
		fsys := s3.NewBucketFS(api, bucket)
		_, err := fsys.WriteWithOptions(ctx, key, strings.NewReader(body), withLength(int64(len(body))+10))
		be.Nonzero(t, err)
		isPathError(t, err)
		var apiErr smithy.APIError
		be.True(t, errors.As(err, &apiErr))
		be.Equal(t, "IncompleteBody", apiErr.ErrorCode())
	})
}

func TestRemove_Mock(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		bucket string
		key    string
		mock   func(*testing.T) *mock.S3API
		// api optionally wraps the mock, for a case that needs a failure
		// the mock does not itself produce.
		api    func(*testing.T, *mock.S3API) s3.S3API
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
				// The probe precedes the delete and names the same key.
				be.AllEqual(t, []string{"remove-me"}, state.KeysFor("HeadObject"))
				be.AllEqual(t, []string{"remove-me"}, state.KeysFor("DeleteObject"))
			},
		}, {
			// DeleteObject is idempotent, so without the HEAD probe this
			// reports success for a key that was never in the bucket --
			// silently changing what a revert means when storage moves
			// from a directory to a bucket.
			desc:   "remove missing key",
			bucket: bucket,
			key:    "never-existed",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{Key: "keep-me"})
			},
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.True(t, errors.Is(err, fs.ErrNotExist))
				var pathErr *fs.PathError
				be.True(t, errors.As(err, &pathErr))
				be.Equal(t, "remove", pathErr.Op)
				be.Equal(t, "never-existed", pathErr.Path)
				// The probe failed, so no delete was ever issued: a
				// DeleteObject here would mean Remove had decided the key
				// was there.
				be.Equal(t, 0, state.CallCount("DeleteObject"))
				// And the cause of the failed probe is still reachable.
				var apiErr smithy.APIError
				be.True(t, errors.As(err, &apiErr))
				be.Equal(t, "NotFound", apiErr.ErrorCode())
			},
		}, {
			// The same, against a store whose missing-key error the SDK
			// cannot resolve to a typed error.
			desc:   "remove missing key, generic not-found style",
			bucket: bucket,
			key:    "never-existed",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket).WithNotFoundStyle(mock.NotFoundStyleGeneric)
			},
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.True(t, errors.Is(err, fs.ErrNotExist))
				be.Equal(t, 0, state.CallCount("DeleteObject"))
			},
		}, {
			// A probe that fails for any other reason is not a missing
			// key, and must not be reported as one.
			desc:   "probe fails for another reason",
			bucket: bucket,
			key:    "remove-me",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{Key: "remove-me"})
			},
			api: func(t *testing.T, m *mock.S3API) s3.S3API {
				return &headErrAPI{S3API: m, err: respErr(403, &smithy.GenericAPIError{Code: "AccessDenied"})}
			},
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.True(t, err != nil)
				be.False(t, errors.Is(err, fs.ErrNotExist))
				be.Equal(t, 0, state.CallCount("DeleteObject"))
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			var client s3.S3API = api
			if tcase.api != nil {
				client = tcase.api(t, api)
			}
			fsys := s3.NewBucketFS(client, tcase.bucket)
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
				// A source well under the CopyObject limit takes the
				// single-request path: no doomed CopyObject followed by a
				// fallback, and no multipart machinery at all.
				be.Equal(t, 1, state.CallCount("CopyObject"))
				be.Equal(t, 0, state.CallCount("CreateMultipartUpload"))
			},
		}, {
			// copy no longer tries CopyObject and inspects the failure text
			// to decide whether to fall back to MultiCopier -- the strategy
			// is chosen up front from the HEAD size. This case pins that
			// removal: even an error worded exactly like the old "copy
			// source is larger than the maximum allowable size" trigger
			// must now propagate as-is, on a source well under the 5 GiB
			// threshold that would otherwise pick MultiCopier on size alone.
			desc: "CopyObject failure is returned as-is, no error-text fallback",
			mock: func(t *testing.T) *mock.S3API {
				api := mock.New(bucket, &mock.Object{
					Key:  "src-file",
					Body: []byte("some content"),
				})
				api.CopyObjectFunc = func(_ context.Context, _ *s3v2.CopyObjectInput, _ ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
					return nil, errors.New("copy source is larger than the maximum allowable size")
				}
				return api
			},
			bucket: bucket,
			src:    "src-file",
			dst:    "dst-file",
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.Nonzero(t, err)
				be.Equal(t, int64(0), size)
				be.Equal(t, 0, state.CallCount("CreateMultipartUpload"))
			},
		}, {
			// regression test: the copy-source header was built with
			// url.QueryEscape, which encodes "/" as %2F (destroying path
			// separators) and space as "+" (which S3 reads literally). The
			// mock's CopyObject splits the header on a literal "/" before
			// percent-decoding, the same way a real bucket does, so it
			// rejects that encoding on its own.
			desc: "copy with space and nested path in source key",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{
					Key:  "dir/sub dir/a file.txt",
					Body: []byte("some content"),
				})
			},
			bucket: bucket,
			src:    "dir/sub dir/a file.txt",
			dst:    "dir2/a file.txt",
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, state.UpdatedETag("dir2/a file.txt"))
				be.Nonzero(t, size)
				be.Equal(t, 1, state.CallCount("CopyObject"))
				be.Equal(t, 0, state.CallCount("CreateMultipartUpload"))
			},
		}, {
			desc: "copy with plus in source key",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{
					Key:  "a+b.txt",
					Body: []byte("some content"),
				})
			},
			bucket: bucket,
			src:    "a+b.txt",
			dst:    "dst+c.txt",
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, state.UpdatedETag("dst+c.txt"))
				be.Nonzero(t, size)
				be.Equal(t, 1, state.CallCount("CopyObject"))
				be.Equal(t, 0, state.CallCount("CreateMultipartUpload"))
			},
		}, {
			desc: "copy with non-ASCII source key",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{
					Key:  "café/日本語.txt",
					Body: []byte("some content"),
				})
			},
			bucket: bucket,
			src:    "café/日本語.txt",
			dst:    "dst-café.txt",
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, state.UpdatedETag("dst-café.txt"))
				be.Nonzero(t, size)
				be.Equal(t, 1, state.CallCount("CopyObject"))
				be.Equal(t, 0, state.CallCount("CreateMultipartUpload"))
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

// TestMultiCopier_Mock drives MultiCopier.Copy directly -- range slicing,
// per-part ETags, CompleteMultipartUpload -- against the mock's real
// validation. copy no longer reaches MultiCopier through a CopyObject
// failure text match (that fallback is gone, see TestCopy_Mock); this is
// where the multipart mechanics stay covered end to end.
func TestMultiCopier_Mock(t *testing.T) {
	ctx := context.Background()
	srcSize := int64(51 * megabyte)
	srcBody := mock.RandBytes(srcSize)
	api := mock.New(bucket, &mock.Object{Key: "src-file", Body: srcBody})
	copier := s3.NewMultiCopier(api, func(mc *s3.MultiCopier) {
		mc.PartSize = partSize
	})
	size, err := copier.Copy(ctx, bucket, "dst-file", "src-file")
	be.NilErr(t, err)
	be.Equal(t, srcSize, size)
	expETag := mock.ETag(srcBody, partSize)
	be.Equal(t, expETag, api.UpdatedETag("dst-file"))
	be.Nonzero(t, api.PartCount())
}

// TestMultiCopierSpecialCharacters_Mock is TestMultiCopier_Mock's
// counterpart for special characters in the source key: it drives
// MultiCopier.Copy directly with a source key containing a space and
// non-ASCII characters, exercising the UploadPartCopy call site's
// copy-source encoding independently of copy()'s CopyObject path.
func TestMultiCopierSpecialCharacters_Mock(t *testing.T) {
	ctx := context.Background()
	srcSize := int64(51 * megabyte)
	srcBody := mock.RandBytes(srcSize)
	const src = "dir/a file+é.txt"
	const dst = "dst/a file+é.txt"
	api := mock.New(bucket, &mock.Object{Key: src, Body: srcBody})
	copier := s3.NewMultiCopier(api, func(mc *s3.MultiCopier) {
		mc.PartSize = partSize
	})
	size, err := copier.Copy(ctx, bucket, dst, src)
	be.NilErr(t, err)
	be.Equal(t, srcSize, size)
	expETag := mock.ETag(srcBody, partSize)
	be.Equal(t, expETag, api.UpdatedETag(dst))
	be.Nonzero(t, api.PartCount())
}

// sizedCopyAPI is the mock with HeadObject's ContentLength overridden, so a
// test can drive copy's size-based strategy choice for a source far larger
// than anything actually worth allocating. UploadPartCopy and
// CompleteMultipartUpload are replaced rather than delegated: the mock's
// real versions slice the actual (tiny) stored body by the declared range
// and validate each part's ETag against ones its own UploadPartCopy stored,
// neither of which holds once the size is faked.
type sizedCopyAPI struct {
	*mock.S3API
	// size replaces HeadObject's ContentLength on the src-file response. A
	// negative size instead sets ContentLength to nil, so a test can drive
	// the missing-content-length guard without a store that actually omits
	// it.
	size int64

	mu     sync.Mutex
	ranges []string // CopySourceRange values UploadPartCopy was called with
}

func (a *sizedCopyAPI) HeadObject(ctx context.Context, in *s3v2.HeadObjectInput,
	opts ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	out, err := a.S3API.HeadObject(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	if a.size < 0 {
		out.ContentLength = nil
	} else {
		out.ContentLength = aws.Int64(a.size)
	}
	return out, nil
}

func (a *sizedCopyAPI) UploadPartCopy(ctx context.Context, in *s3v2.UploadPartCopyInput,
	opts ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error) {
	a.mu.Lock()
	a.ranges = append(a.ranges, aws.ToString(in.CopySourceRange))
	a.mu.Unlock()
	return &s3v2.UploadPartCopyOutput{
		CopyPartResult: &types.CopyPartResult{ETag: aws.String(fmt.Sprintf("etag-%d", aws.ToInt32(in.PartNumber)))},
	}, nil
}

func (a *sizedCopyAPI) CompleteMultipartUpload(ctx context.Context, in *s3v2.CompleteMultipartUploadInput,
	opts ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	a.S3API.MPUComplete = true
	return &s3v2.CompleteMultipartUploadOutput{Bucket: in.Bucket, Key: in.Key}, nil
}

// TestCopyStrategy_Mock covers the behavior change directly: the strategy is
// read from the source's HEAD size, not discovered by trying CopyObject and
// inspecting the failure.
func TestCopyStrategy_Mock(t *testing.T) {
	ctx := context.Background()
	const maxCopySize = 5 * 1024 * int64(megabyte) // CopyObject's own limit

	t.Run("exactly the limit takes the single-request path", func(t *testing.T) {
		base := mock.New(bucket, &mock.Object{Key: "src-file", Body: []byte("some content")})
		api := &sizedCopyAPI{S3API: base, size: maxCopySize}
		fsys := s3.NewBucketFS(api, bucket)
		size, err := fsys.Copy(ctx, "dst-file", "src-file")
		be.NilErr(t, err)
		be.Equal(t, maxCopySize, size)
		be.Equal(t, 1, api.CallCount("CopyObject"))
		be.Equal(t, 0, api.CallCount("CreateMultipartUpload"))
	})

	t.Run("one byte over the limit takes the multipart path", func(t *testing.T) {
		base := mock.New(bucket, &mock.Object{Key: "src-file", Body: []byte("some content")})
		api := &sizedCopyAPI{S3API: base, size: maxCopySize + 1}
		fsys := s3.NewBucketFS(api, bucket)
		size, err := fsys.Copy(ctx, "dst-file", "src-file")
		be.NilErr(t, err)
		be.Equal(t, maxCopySize+1, size)
		// The point of the change: no CopyObject is attempted at all, let
		// alone one that is left to fail first.
		be.Equal(t, 0, api.CallCount("CopyObject"))
		be.Equal(t, 1, api.CallCount("CreateMultipartUpload"))
		be.True(t, api.MPUCompleteFlag())

		// The parts must tile [0, size) with no gap or overlap.
		type span struct{ start, end int64 }
		spans := make([]span, len(api.ranges))
		for i, r := range api.ranges {
			var sp span
			n, err := fmt.Sscanf(r, "bytes=%d-%d", &sp.start, &sp.end)
			be.NilErr(t, err)
			be.Equal(t, 2, n)
			spans[i] = sp
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
		var next int64
		for _, sp := range spans {
			be.Equal(t, next, sp.start)
			next = sp.end + 1
		}
		be.Equal(t, maxCopySize+1, next)
	})

	t.Run("nil ContentLength is refused, not dereferenced", func(t *testing.T) {
		base := mock.New(bucket, &mock.Object{Key: "src-file", Body: []byte("some content")})
		api := &sizedCopyAPI{S3API: base, size: -1}
		fsys := s3.NewBucketFS(api, bucket)
		_, err := fsys.Copy(ctx, "dst-file", "src-file")
		be.Nonzero(t, err)
		be.Equal(t, 0, api.CallCount("CopyObject"))
	})
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

func (stubClient) DeleteObjects(context.Context, *s3v2.DeleteObjectsInput, ...func(*s3v2.Options)) (*s3v2.DeleteObjectsOutput, error) {
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

// failDeleteAPI is the mock with DeleteObjects made to report a failure for
// one chosen key. The failure is not a transport error: DeleteObjects answers
// 200 and names the key in the response's Errors list, which is how a real
// endpoint reports a key that would not delete. Every other key in the batch
// deletes as usual.
//
// The mock records calls but injects no errors, and does not need to: the
// method set it satisfies is an interface, so shadowing one method on an
// embedding type is enough to stand in for the whole thing.
type failDeleteAPI struct {
	*mock.S3API
	failKey string
	code    string
	message string
	// pageSize, when > 0, clamps the listing page size so the keys span
	// several pages -- enough to show that a page whose delete reported a
	// failure is still followed by the next one.
	pageSize int32
	// attempted records every key a delete request carried. The mock's own
	// call log cannot serve here: the failing key is filtered out before
	// the request reaches it.
	attempted []string
}

func (a *failDeleteAPI) ListObjectsV2(ctx context.Context, in *s3v2.ListObjectsV2Input,
	opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	if a.pageSize <= 0 {
		return a.S3API.ListObjectsV2(ctx, in, opts...)
	}
	req := *in
	req.MaxKeys = aws.Int32(a.pageSize)
	return a.S3API.ListObjectsV2(ctx, &req, opts...)
}

func (a *failDeleteAPI) DeleteObjects(ctx context.Context, in *s3v2.DeleteObjectsInput,
	opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectsOutput, error) {
	kept := make([]types.ObjectIdentifier, 0, len(in.Delete.Objects))
	var failed *types.Error
	for _, obj := range in.Delete.Objects {
		key := aws.ToString(obj.Key)
		a.attempted = append(a.attempted, key)
		if key == a.failKey {
			failed = &types.Error{
				Key:     obj.Key,
				Code:    aws.String(a.code),
				Message: aws.String(a.message),
			}
			continue
		}
		kept = append(kept, obj)
	}
	if failed == nil {
		return a.S3API.DeleteObjects(ctx, in, opts...)
	}
	if len(kept) == 0 {
		// Nothing left to send: a real endpoint would still answer 200 with
		// the one failure, and the mock rejects an empty request.
		return &s3v2.DeleteObjectsOutput{Errors: []types.Error{*failed}}, nil
	}
	req := *in
	del := *in.Delete
	del.Objects = kept
	req.Delete = &del
	out, err := a.S3API.DeleteObjects(ctx, &req, opts...)
	if err != nil {
		return out, err
	}
	out.Errors = append(out.Errors, *failed)
	return out, nil
}

// pageAPI is the mock with a chosen listing page size, so a test can exercise
// the paging and batching paths without seeding thousands of objects: the
// mock already honors MaxKeys and continuation tokens, so clamping the page
// size drives the same loop a full bucket would.
//
// It also records every DeleteObjects request it served, which is how a test
// sees the request shape -- the batch each one carried, and whether it asked
// for quiet mode -- and can fail a chosen one at the transport level.
type pageAPI struct {
	*mock.S3API
	pageSize int32
	// failCall, when > 0, makes the DeleteObjects call with that 1-based
	// number fail outright, the way a dropped connection would.
	failCall int
	failErr  error

	deletes []*s3v2.DeleteObjectsInput
}

var _ s3.S3API = (*pageAPI)(nil)

func (a *pageAPI) ListObjectsV2(ctx context.Context, in *s3v2.ListObjectsV2Input,
	opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	req := *in
	req.MaxKeys = aws.Int32(a.pageSize)
	return a.S3API.ListObjectsV2(ctx, &req, opts...)
}

func (a *pageAPI) DeleteObjects(ctx context.Context, in *s3v2.DeleteObjectsInput,
	opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectsOutput, error) {
	a.deletes = append(a.deletes, in)
	if a.failCall > 0 && len(a.deletes) == a.failCall {
		return nil, a.failErr
	}
	return a.S3API.DeleteObjects(ctx, in, opts...)
}

// TestRemoveAllBatches_Mock covers how removeAll issues its deletes. Listing a
// page and then deleting one key per request cost a round trip per file: an
// OCFL object of 10,000 files took 10,000 sequential DeleteObject calls where
// DeleteObjects takes a whole page at once.
//
// KeyBatchesFor is the assertion that catches a regression back to per-key
// deletes. KeysFor cannot: 500 keys in one batch and 500 separate calls name
// exactly the same keys.
func TestRemoveAllBatches_Mock(t *testing.T) {
	ctx := context.Background()

	t.Run("one request per page, quietly", func(t *testing.T) {
		keys := []string{"d/a", "d/b", "d/c", "d/d", "d/e"}
		objects := make([]*mock.Object, len(keys))
		for i, key := range keys {
			objects[i] = &mock.Object{Key: key, Body: []byte(key)}
		}
		api := &pageAPI{S3API: mock.New(bucket, objects...), pageSize: 2}
		fsys := s3.NewBucketFS(api, bucket)
		be.NilErr(t, fsys.RemoveAll(ctx, "d"))

		// Three pages of two, two, one -- one delete request each, carrying
		// that page's keys and nothing else.
		be.DeepEqual(t, [][]string{{"d/a", "d/b"}, {"d/c", "d/d"}, {"d/e"}},
			api.KeyBatchesFor("DeleteObjects"))
		// And not one per-key delete anywhere.
		be.Equal(t, 0, api.CallCount("DeleteObject"))

		// Quiet mode: the response then carries only the failures, which is
		// all removeAll reads.
		for _, in := range api.deletes {
			be.True(t, aws.ToBool(in.Delete.Quiet))
		}

		// The keys are actually gone, not merely named in a request.
		for _, key := range keys {
			be.True(t, api.WasDeleted(key))
		}
	})

	// The page size and the batch limit are separate numbers that happen to
	// be equal today. A page larger than a batch must still be split, so a
	// future page-size bump cannot silently send a request S3 rejects
	// outright -- the mock enforces the limit the way a real endpoint does.
	t.Run("a page larger than the batch limit is split", func(t *testing.T) {
		const numKeys = mock.MaxDeleteBatch + 200
		objects := make([]*mock.Object, numKeys)
		for i := range objects {
			objects[i] = &mock.Object{Key: fmt.Sprintf("d/%05d", i)}
		}
		api := &pageAPI{S3API: mock.New(bucket, objects...), pageSize: numKeys}
		fsys := s3.NewBucketFS(api, bucket)
		be.NilErr(t, fsys.RemoveAll(ctx, "d"))

		batches := api.KeyBatchesFor("DeleteObjects")
		be.Equal(t, 2, len(batches))
		be.Equal(t, mock.MaxDeleteBatch, len(batches[0]))
		be.Equal(t, 200, len(batches[1]))
		be.Equal(t, numKeys, len(api.KeysFor("DeleteObjects")))
	})

	// DeleteObjects answers 200 even when individual keys fail, reporting
	// them in the response body. Reading only the transport error would call
	// a RemoveAll successful while the prefix is still partly populated.
	t.Run("per-key failures are reported", func(t *testing.T) {
		api := &failDeleteAPI{
			S3API:   mock.New(bucket, &mock.Object{Key: "d/a"}, &mock.Object{Key: "d/b"}),
			failKey: "d/b",
			code:    "AccessDenied",
			message: "Access Denied",
		}
		fsys := s3.NewBucketFS(api, bucket)

		err := fsys.RemoveAll(ctx, "d")
		be.True(t, err != nil)
		// The error names the key that failed, not the name RemoveAll was
		// called with, and carries the reason the response gave.
		be.True(t, strings.Contains(err.Error(), "d/b"))
		be.True(t, strings.Contains(err.Error(), "AccessDenied"))
		be.False(t, strings.Contains(err.Error(), "d/a"))
		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "removeall", pathErr.Op)
		be.Equal(t, "d/b", pathErr.Path)

		// The rest of the batch still deleted.
		be.True(t, api.WasDeleted("d/a"))
		be.False(t, api.WasDeleted("d/b"))
	})

	// A DeleteObjects that fails at the transport level says nothing about
	// individual keys, so it is reported once against the name the caller
	// passed -- and, being best-effort, does not stop the next page.
	t.Run("a failed request does not stop the walk", func(t *testing.T) {
		boom := errors.New("boom")
		keys := []string{"d/a", "d/b", "d/c", "d/d"}
		objects := make([]*mock.Object, len(keys))
		for i, key := range keys {
			objects[i] = &mock.Object{Key: key}
		}
		api := &pageAPI{
			S3API:    mock.New(bucket, objects...),
			pageSize: 2,
			failCall: 1,
			failErr:  boom,
		}
		fsys := s3.NewBucketFS(api, bucket)

		err := fsys.RemoveAll(ctx, "d")
		be.True(t, errors.Is(err, boom))

		// Page 2 was listed and deleted anyway. The count comes from the
		// wrapper's own record: the failing request never reaches the mock,
		// so the mock's call log cannot see it.
		be.Equal(t, 2, len(api.deletes))
		be.False(t, api.WasDeleted("d/a"))
		be.False(t, api.WasDeleted("d/b"))
		be.True(t, api.WasDeleted("d/c"))
		be.True(t, api.WasDeleted("d/d"))
	})
}

// TestRemoveAllBestEffort pins the WriteFS.RemoveAll contract that one key
// which will not delete must not abandon the keys after it. The subtree case
// is the one this changed: removeAll used to return on the first failing
// delete, leaving every later key in place and reporting only that one
// failure. Batching does not weaken it -- a page whose response reports a
// failure must still be followed by the next page.
func TestRemoveAllBestEffort(t *testing.T) {
	ctx := context.Background()

	// Keys are chosen so the failing one sorts in the middle: a listing is
	// returned in key order, so a survivor after it is what proves the loop
	// kept going.
	for _, tc := range []struct {
		name    string
		remove  string
		keys    []string
		failKey string
		// pageSize, when set, splits the keys across several listing pages
		// so the failure lands on a page with pages after it.
		pageSize int32
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
		{
			name:     "across pages",
			remove:   "dir",
			keys:     []string{"dir/a.txt", "dir/b.txt", "dir/c.txt", "dir/d.txt"},
			failKey:  "dir/a.txt",
			pageSize: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := make([]*mock.Object, 0, len(tc.keys))
			for _, key := range tc.keys {
				objects = append(objects, &mock.Object{Key: key, Body: []byte(key)})
			}
			api := &failDeleteAPI{
				S3API:    mock.New(bucket, objects...),
				failKey:  tc.failKey,
				code:     "InternalError",
				message:  "boom",
				pageSize: tc.pageSize,
			}
			fsys := s3.NewBucketFS(api, bucket)

			err := fsys.RemoveAll(ctx, tc.remove)
			be.True(t, err != nil)
			be.True(t, strings.Contains(err.Error(), "boom"))
			be.True(t, strings.Contains(err.Error(), tc.failKey))

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

// nilHeadFieldAPI is the mock with chosen fields on HeadObject's response
// nilled out, so a test can drive openFile's missing-ContentLength (or
// missing-LastModified) guard without a store that actually omits the
// header.
type nilHeadFieldAPI struct {
	*mock.S3API
	nilContentLength bool
	nilLastModified  bool
}

func (a *nilHeadFieldAPI) HeadObject(ctx context.Context, in *s3v2.HeadObjectInput,
	opts ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	out, err := a.S3API.HeadObject(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	if a.nilContentLength {
		out.ContentLength = nil
	}
	if a.nilLastModified {
		out.LastModified = nil
	}
	return out, nil
}

// TestOpenNilHeadField_Mock covers the openFile side of the nil-HEAD-field
// guard: OpenFile refuses a HEAD response missing ContentLength or
// LastModified instead of handing back a File whose Stat, Read or Seek would
// nil-deref the first time they need the value.
func TestOpenNilHeadField_Mock(t *testing.T) {
	ctx := context.Background()
	obj := &mock.Object{Key: "src-file", Body: []byte("some content"), LastModified: time.Now()}

	for _, tc := range []struct {
		desc             string
		nilContentLength bool
		nilLastModified  bool
	}{
		{"nil ContentLength", true, false},
		{"nil LastModified", false, true},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			base := mock.New(bucket, obj)
			api := &nilHeadFieldAPI{
				S3API:            base,
				nilContentLength: tc.nilContentLength,
				nilLastModified:  tc.nilLastModified,
			}
			fsys := s3.NewBucketFS(api, bucket)
			f, err := fsys.OpenFile(ctx, obj.Key)
			be.Zero(t, f)
			isPathError(t, err)
			// The object is there; the store just didn't say how big (or how
			// recently modified) it is. A caller must not mistake that for a
			// missing key.
			be.False(t, errors.Is(err, fs.ErrNotExist))
			be.Equal(t, 0, api.CallCount("GetObject"))
		})
	}
}
