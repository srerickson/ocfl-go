package s3_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/backend/s3"
)

const (
	testBucketPrefix = "ocfl-go-test-"
	defaultRegion    = "us-east-1"
)

func TestWrite(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	_, err := b.Write(ctx, "test/test3", strings.NewReader("hello"))
	be.NilErr(t, err)
	f, err := b.OpenFile(ctx, "test/test3")
	be.NilErr(t, err)
	got, err := io.ReadAll(f)
	be.NilErr(t, err)
	be.Equal(t, "hello", string(got))
}

func TestReadDir(t *testing.T) {
	b := newBackend(t)
	ctx := context.Background()
	t.Run("large directory", func(t *testing.T) {
		const num = 1_001
		const dir = "large-dir"
		for i := 0; i < num; i++ {
			key := fmt.Sprintf("%s/file-%d.txt", dir, i)
			_, err := b.Write(ctx, key, strings.NewReader(""))
			be.NilErr(t, err)
			key = fmt.Sprintf("%s/dir-%d/file.txt", dir, i)
			_, err = b.Write(ctx, key, strings.NewReader(""))
			be.NilErr(t, err)
		}
		entries, err := b.ReadDir(ctx, dir)
		be.NilErr(t, err)
		be.Equal(t, 2*num, len(entries))
		be.True(t, slices.IsSortedFunc(entries, func(a, b fs.DirEntry) int {
			return strings.Compare(a.Name(), b.Name())
		}))
	})
}

func TestCopy(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	t.Run("large file", func(t *testing.T) {
		src := "large-file"
		dst := "new-file"
		size := int64(1024 * 1024 * 1024 * 50)
		t.Log("doing write")
		_, err := b.Write(ctx, src, io.LimitReader(rand.Reader, size))
		be.NilErr(t, err)
		t.Log("doing copy")
		be.NilErr(t, b.Copy(ctx, dst, src))
		f, err := b.OpenFile(ctx, dst)
		be.NilErr(t, err)
		defer f.Close()
		t.Log("doing stat")
		info, err := f.Stat()
		be.NilErr(t, err)
		be.Equal(t, size, info.Size())
	})
}

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
	be.NilErr(t, err)
	t.Log("created test bucket", testBucket)
	t.Cleanup(func() {
		be.NilErr(t, destroyBucket(ctx, s3client, testBucket))
		t.Log("removed test bucket", testBucket)
	})
	return s3.New(cfg, testBucket)
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
