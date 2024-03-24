package s3_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/backend/s3"
	"github.com/srerickson/ocfl-go/backend/s3/internal/mock"
)

const (
	testBucketPrefix = "ocfl-go-test-"
	defaultRegion    = "us-east-1"
	bucket           = "ocfl-go-test"
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
		},
		{
			desc:   "missing key return ErrNotExist",
			key:    "missing",
			bucket: bucket,
			mock: func(t *testing.T) s3.OpenFileAPI {
				return mock.OpenFileAPI(t, bucket)
			},
			expect: func(t *testing.T, _ fs.File, err error) {
				isPathError(t, err)
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
		{
			desc: "key '.' is invalid",
			key:  ".",
			expect: func(t *testing.T, _ fs.File, err error) {
				isPathError(t, err)
				be.True(t, errors.Is(err, fs.ErrInvalid))
			},
		},
		{
			desc: "key with .. is invalid",
			key:  "../invalid",
			expect: func(t *testing.T, _ fs.File, err error) {
				isPathError(t, err)
				be.True(t, errors.Is(err, fs.ErrInvalid))
			},
		},
	}

	for i, tcase := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var api s3.OpenFileAPI
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			f, err := s3.OpenFile(ctx, api, tcase.bucket, tcase.key)
			tcase.expect(t, f, err)
		})
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

// func TestWrite(t *testing.T) {
// 	ctx := context.Background()
// 	b := newBackend(t)
// 	_, err := b.Write(ctx, "test/test3", strings.NewReader("hello"))
// 	be.NilErr(t, err)
// 	f, err := b.OpenFile(ctx, "test/test3")
// 	be.NilErr(t, err)
// 	got, err := io.ReadAll(f)
// 	be.NilErr(t, err)
// 	be.Equal(t, "hello", string(got))
// }

// func TestReadDir(t *testing.T) {
// 	b := newBackend(t)
// 	ctx := context.Background()
// 	t.Run("large directory", func(t *testing.T) {
// 		const num = 1_001
// 		const dir = "large-dir"
// 		for i := 0; i < num; i++ {
// 			key := fmt.Sprintf("%s/file-%d.txt", dir, i)
// 			_, err := b.Write(ctx, key, strings.NewReader(""))
// 			be.NilErr(t, err)
// 			key = fmt.Sprintf("%s/dir-%d/file.txt", dir, i)
// 			_, err = b.Write(ctx, key, strings.NewReader(""))
// 			be.NilErr(t, err)
// 		}
// 		entries, err := b.ReadDir(ctx, dir)
// 		be.NilErr(t, err)
// 		be.Equal(t, 2*num, len(entries))
// 		be.True(t, slices.IsSortedFunc(entries, func(a, b fs.DirEntry) int {
// 			return strings.Compare(a.Name(), b.Name())
// 		}))
// 	})
// }

// func TestCopy(t *testing.T) {
// 	ctx := context.Background()
// 	b := newBackend(t)
// 	t.Run("large file", func(t *testing.T) {
// 		src := "large-file"
// 		dst := "new-file"
// 		size := int64(1024 * 1024 * 1024 * 10)
// 		t.Log("doing write")
// 		_, err := b.Write(ctx, src, io.LimitReader(rand.Reader, size))
// 		be.NilErr(t, err)
// 		t.Log("doing copy")
// 		be.NilErr(t, b.Copy(ctx, dst, src))
// 		f, err := b.OpenFile(ctx, dst)
// 		be.NilErr(t, err)
// 		defer f.Close()
// 		t.Log("doing stat")
// 		info, err := f.Stat()
// 		be.NilErr(t, err)
// 		be.Equal(t, size, info.Size())
// 	})
// }

func newBackend(t *testing.T) *s3.FS {
	ctx := context.Background()
	// creds := credentials.NewStaticCredentialsProvider("", "", "")
	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				PartitionID:       "aws",
				URL:               "http://localhost:9000",
				SigningRegion:     defaultRegion,
				HostnameImmutable: true,
			}, nil
		})
	opts := []func(*config.LoadOptions) error{
		config.WithDefaultRegion(defaultRegion),
		// config.WithCredentialsProvider(creds),
		config.WithEndpointResolverWithOptions(customResolver),
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		t.Fatal(err)
	}
	testBucket := randName(testBucketPrefix)
	s3client := s3v2.NewFromConfig(cfg)
	_, err = s3client.CreateBucket(ctx, &s3v2.CreateBucketInput{
		Bucket: aws.String(testBucket),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("created test bucket", testBucket)
	t.Cleanup(func() {
		if err := destroyBucket(ctx, s3client, testBucket); err != nil {
			t.Fatal(err)
		}
		t.Log("removed test bucket", testBucket)
	})
	return &s3.FS{
		S3:     s3client,
		Bucket: testBucket,
	}
}

func destroyBucket(ctx context.Context, s3cl *s3v2.Client, bucket string) error {
	b := aws.String(bucket)
	listopts := &s3v2.ListObjectsV2Input{Bucket: b}
	for {
		list, err := s3cl.ListObjectsV2(ctx, listopts)
		if err != nil {
			return err
		}
		for _, obj := range list.Contents {
			if _, err := s3cl.DeleteObject(ctx, &s3v2.DeleteObjectInput{
				Bucket: b,
				Key:    obj.Key,
			}); err != nil {
				return err
			}
		}
		if list.IsTruncated != nil && !*list.IsTruncated {
			break
		}
		listopts.ContinuationToken = list.NextContinuationToken
	}
	_, err := s3cl.DeleteBucket(ctx, &s3v2.DeleteBucketInput{Bucket: b})
	return err
}

func randName(prefix string) string {
	byt, err := io.ReadAll(io.LimitReader(rand.Reader, 8))
	if err != nil {
		panic("randName: " + err.Error())
	}
	return prefix + hex.EncodeToString(byt)
}
