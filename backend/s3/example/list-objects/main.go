package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/s3"
)

func main() {
	flag.Parse()
	ctx := context.Background()
	bucket := flag.Arg(0)
	prefix := flag.Arg(1)
	if bucket == "" {
		log.Fatal("missing arg: bucket name")
	}
	if prefix == "" {
		log.Fatal("missing arg: prefix")
	}
	if err := run(ctx, bucket, prefix); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, bucket string, prefix string) error {
	fsys, err := backend(ctx, bucket)
	if err != nil {
		return err
	}
	var iterErr error
	ocfl.ObjectRoots(ctx, fsys, prefix)(func(obj *ocfl.ObjectRoot, err error) bool {
		if err != nil {
			iterErr = err
			return false
		}
		fmt.Println(obj.Path)
		return true
	})
	return iterErr
}

func backend(ctx context.Context, bucket string) (*s3.BucketFS, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &s3.BucketFS{
		S3:     s3v2.NewFromConfig(cfg),
		Bucket: bucket,
	}, nil
}
