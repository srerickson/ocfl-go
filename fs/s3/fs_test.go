package s3_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"iter"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"

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
				be.Nonzero(t, state.UpdatedETags["dst-file"])
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
