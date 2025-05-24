package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go/fs"
	ocflS3 "github.com/srerickson/ocfl-go/fs/s3"
)

const (
	envS3Enabled = "OCFL_TEST_S3"
	tmpPrefix    = "ocfl-go-test"
)

// S3Enabled returns true if $OCFL_TEST_S3 is set
func S3Enabled() bool { return os.Getenv(envS3Enabled) != "" }

func S3Client(ctx context.Context) (*s3.Client, error) {
	endpoint := os.Getenv(envS3Enabled)
	if endpoint == "" {
		return nil, errors.New("S3 not enabled in thest test environment: $OCFL_TEST_S3 not set")
	}
	cnf, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	cli := s3.NewFromConfig(cnf, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return cli, nil
}

// TmpS3FS returns an *ocflS3.BucketFS backed by a tempory S3 bucket. The bucket
// and its contents will be erased as part of the test's cleanup.
func TmpS3FS(t *testing.T, testdata fs.FS) *ocflS3.BucketFS {
	t.Helper()
	ctx := context.Background()
	cli, err := S3Client(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bucket, err := TmpBucket(ctx, cli)
	if err != nil {
		t.Fatal("setting up S3 bucket:", err)
	}
	t.Cleanup(func() {
		if err := RemoveBucket(ctx, cli, bucket); err != nil {
			t.Fatal("cleaning up S3 bucket:", err)
		}
	})
	s3fs := &ocflS3.BucketFS{S3: cli, Bucket: bucket}
	if testdata != nil {
		files := fs.WalkFiles(ctx, testdata, ".")
		for file, err := range files {
			if err != nil {
				t.Fatal("reading from testdata:", err)
			}
			name := file.FullPath()
			if _, err := fs.Copy(ctx, s3fs, name, testdata, name); err != nil {
				t.Fatal("copying testdata to tmp S3 bucket: %w", err)
			}
		}
	}

	return s3fs
}

func TmpBucket(ctx context.Context, cli *s3.Client) (string, error) {
	var bucket string
	var retries int
	for {
		bucket = randName(tmpPrefix)
		_, err := cli.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &bucket})
		if err == nil {
			break
		}
		if retries > 4 {
			return "", err
		}
		retries++
	}
	return bucket, nil
}

func RemoveBucket(ctx context.Context, s3cl *s3.Client, bucket string) error {
	if !strings.HasPrefix(bucket, tmpPrefix) {
		return errors.New("bucket name doesn't look like a test bucket: " + bucket)
	}
	b := aws.String(bucket)
	listInput := &s3.ListObjectsV2Input{Bucket: b}
	for {
		list, err := s3cl.ListObjectsV2(ctx, listInput)
		if err != nil {
			return err
		}
		for _, obj := range list.Contents {
			_, err = s3cl.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: b,
				Key:    obj.Key,
			})
			if err != nil {
				return err
			}
		}
		listInput.ContinuationToken = list.NextContinuationToken
		if listInput.ContinuationToken == nil {
			break
		}
	}
	_, err := s3cl.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

func randName(prefix string) string {
	byt, err := io.ReadAll(io.LimitReader(rand.Reader, 4))
	if err != nil {
		panic("randName: " + err.Error())
	}
	now := time.Now().UnixMicro()
	return fmt.Sprintf("%s-%d-%s", prefix, now, hex.EncodeToString(byt))
}
