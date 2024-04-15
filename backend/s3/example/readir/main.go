package main

import (
	"context"
	"errors"
	"flag"

	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go/backend/s3"
)

func main() {
	ctx := context.Background()
	flag.Parse()
	bucket := flag.Arg(0)
	prefix := flag.Arg(1)
	if err := run(ctx, bucket, prefix); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, bucket, prefix string) error {
	if bucket == "" {
		return errors.New("bucket param is required")
	}
	if prefix == "" {
		prefix = "."
	}
	fsys, err := backend(ctx, bucket)
	if err != nil {
		return err
	}
	entries, err := fsys.ReadDir(ctx, prefix)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		// fmt.Println(name)
	}
	return nil
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
