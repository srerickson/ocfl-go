package s3_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/backend/s3"
	"github.com/srerickson/ocfl-go/backend/s3/internal/mock"
)

const (
	testBucketPrefix = "ocfl-go-test-"
	defaultRegion    = "us-east-1"
	bucket           = "ocfl-go-test"
	megabyte         = 1024 * 1024
	gigabyte         = 1024 * megabyte
	mockSeed         = 1288108737
)

func TestOpenFile(t *testing.T) {
	type testCase struct {
		desc   string
		bucket string
		key    string
		mock   func(*testing.T) s3.OpenFileAPI
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
			mock: func(t *testing.T) s3.OpenFileAPI {
				return mock.OpenFileAPI(t, bucket, obj)
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
			mock: func(t *testing.T) s3.OpenFileAPI {
				return mock.OpenFileAPI(t, bucket)
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
			var api s3.OpenFileAPI
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			f, err := s3.OpenFile(ctx, api, tcase.bucket, tcase.key)
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
		mock   func(*testing.T) s3.ReadDirAPI
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
			desc:   "2k files",
			bucket: bucket,
			dir:    "tmp",
			mock: func(t *testing.T) s3.ReadDirAPI {
				return mock.ReadDirAPI(t, bucket, mock.GenObjects(mockSeed, 2001, "tmp", 1, 100)...)
			},
			expect: func(t *testing.T, entries []fs.DirEntry, err error) {
				be.NilErr(t, err)
				be.Equal(t, 2001, len(entries))
				be.True(t, !slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
					return e.IsDir()
				}))
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api s3.ReadDirAPI
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			entries, err := s3.ReadDir(ctx, api, tcase.bucket, tcase.dir)
			tcase.expect(t, entries, err)
		})
	}
}

func TestWrite(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		bucket string
		conc   int
		psize  int64
		key    string
		body   io.Reader
		mock   func(*testing.T) s3.WriteAPI
		expect func(*testing.T, int64, error)
	}
	cases := []testCase{
		{
			desc: "invalid path",
			key:  "../file.txt",
			expect: func(t *testing.T, size int64, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc:   "small write",
			bucket: bucket,
			key:    "tmp",
			body:   strings.NewReader("some content"),
			mock: func(t *testing.T) s3.WriteAPI {
				return mock.WriteAPI(t, bucket)
			},
			expect: func(t *testing.T, size int64, err error) {
				be.NilErr(t, err)
			},
		}, {
			desc:   "multipart",
			bucket: bucket,
			key:    "tmp",
			psize:  5 * megabyte,
			body:   io.LimitReader(rand.Reader, 500*megabyte),
			mock: func(t *testing.T) s3.WriteAPI {
				api := mock.WriteAPI(t, bucket)
				api.Put = func(_ context.Context, _ *s3v2.PutObjectInput, _ ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
					t.Error("PutObject shouldn't be called")
					return nil, errors.New("fail")
				}
				return api
			},
			expect: func(t *testing.T, size int64, err error) {
				be.Equal(t, 500*megabyte, size)
				be.NilErr(t, err)
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api s3.WriteAPI
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			val, err := s3.Write(ctx, api, tcase.bucket, tcase.conc, tcase.psize, tcase.key, tcase.body)
			tcase.expect(t, val, err)
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

func randName(prefix string) string {
	byt, err := io.ReadAll(io.LimitReader(rand.Reader, 8))
	if err != nil {
		panic("randName: " + err.Error())
	}
	return prefix + hex.EncodeToString(byt)
}
