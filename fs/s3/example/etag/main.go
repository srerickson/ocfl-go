package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

const (
	megabyte int64 = 1024 * 1024
	psize          = 6 * megabyte
	conc           = 3
)

func main() {
	ctx := context.Background()
	flag.Parse()
	bucket := flag.Arg(0)
	key := flag.Arg(1)
	if err := run(ctx, bucket, key); err != nil {
		log.Fatal(err)
	}
}

// Do a multipart write to S3, get the ETag for the
// new object and compare to value generated by mock.ETag()
func run(ctx context.Context, bucket, key string) error {
	fsys, err := backend(ctx, bucket)
	if err != nil {
		return err
	}
	const size = 51 * megabyte
	const seed uint64 = 1929
	buf := mock.RandBytes(seed, size)
	if _, err := fsys.Write(ctx, key, bytes.NewReader(buf)); err != nil {
		return err
	}
	expect := mock.ETag(buf, psize)
	obj, err := fsys.S3.GetObject(ctx, &s3v2.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return err
	}
	if expect != *obj.ETag {
		fmt.Printf("ETag doesn't match expected valued: %q != %q\n", *obj.ETag, expect)
	}
	_, err = fsys.S3.DeleteObject(ctx, &s3v2.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	return err
}

func backend(ctx context.Context, bucket string) (*s3.BucketFS, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &s3.BucketFS{
		S3:                    s3v2.NewFromConfig(cfg),
		Bucket:                bucket,
		DefaultUploadPartSize: psize,
		UploadConcurrency:     conc,
	}, nil
}
