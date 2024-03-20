package main

import (
	"context"
	// "crypto/rand"
	"fmt"
	// "io"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/srerickson/ocfl-go/backend/s3"
)

const (
	gigabyte int64 = 1024 * 1024 * 1024
	bucket         = "ocfl-go-test"
)

func main() {
	ctx := context.Background()
	if err := run(ctx, bucket); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, bucket string) error {
	b, err := backend(ctx, bucket)
	if err != nil {
		return err
	}
	src := "test"
	dst := "new-file"
	// body := io.LimitReader(rand.Reader, 7*gigabyte)
	// fmt.Print("writing large file...")
	// if _, err := b.Write(ctx, src, body); err != nil {
	// 	return err
	// }
	// fmt.Println("done")
	fmt.Print("copying large file...")
	if err := b.MultipartCopy(ctx, dst, src); err != nil {
		return err
	}
	fmt.Println("done")
	return nil
}

func backend(ctx context.Context, bucket string) (*s3.FS, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s3.New(cfg, bucket), nil
}
