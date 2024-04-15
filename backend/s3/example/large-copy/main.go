package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/s3"
)

const (
	megabyte int64 = 1024 * 1024
	gigabyte       = 1024 * megabyte
	bucket         = "ocfl-go-test"
)

var (
	srcSize   = flag.Int("size", 16, "source file size (GiB)")
	copyTests = []copyTest{
		{conc: 15, psize: 5},
		{conc: 15, psize: 50},
		{conc: 15, psize: 500},

		{conc: 25, psize: 5},
		{conc: 25, psize: 50},
		{conc: 25, psize: 500},

		// {conc: 30, psize: 5},
		// {conc: 30, psize: 50},
		// {conc: 30, psize: 500},
	}
)

type copyTest struct {
	conc  int
	psize int
}

func main() {
	flag.Parse()
	ctx := context.Background()
	size := int64(*srcSize) * gigabyte
	if err := doTests(ctx, bucket, size); err != nil {
		log.Fatal(err)
	}
}

func doTests(ctx context.Context, bucket string, size int64) error {
	fmt.Println("source_size,part_concurrency,part_size,copy_time")
	b, err := backend(ctx, bucket)
	if err != nil {
		return err
	}
	src := fmt.Sprintf("copy-source-%d", size/gigabyte)
	if err := createSrcFile(ctx, b, src, size); err != nil {
		return err
	}
	for _, t := range copyTests {
		b.DefaultCopyPartSize = int64(t.psize) * megabyte
		b.CopyPartConcurrency = t.conc
		start := time.Now()
		dst := fmt.Sprintf("copy-conc=%d-psize=%d", b.CopyPartConcurrency, b.DefaultCopyPartSize)
		// fmt.Fprintln(os.Stderr, dst)
		if err := b.Copy(ctx, dst, src); err != nil {
			return err
		}

		fmt.Printf("%d, %d, %d, %0.2f\n",
			size/gigabyte,
			b.CopyPartConcurrency,
			b.DefaultCopyPartSize/megabyte,
			time.Since(start).Seconds())
		if err := b.Remove(ctx, dst); err != nil {
			return err
		}
	}
	return nil
}

func createSrcFile(ctx context.Context, fsys ocfl.WriteFS, key string, size int64) error {
	if f, err := fsys.OpenFile(ctx, key); err == nil {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		f.Close()
		if info.Size() == size {
			return nil
		}
	}
	fmt.Fprintln(os.Stderr, "writing source file")
	body := io.LimitReader(rand.Reader, size)
	_, err := fsys.Write(ctx, key, body)
	return err
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
