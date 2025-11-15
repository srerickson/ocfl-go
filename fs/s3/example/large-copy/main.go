package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go/fs/s3"
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
	src, err := createSrcFile(ctx, bucket, size)
	if err != nil {
		return err
	}
	for _, t := range copyTests {
		copyOpts := func(mc *s3.MultiCopier) {
			mc.Concurrency = t.conc
			mc.PartSize = int64(t.psize) * megabyte
		}
		fsys, err := backend(ctx, bucket, s3.WithMultiPartCopyOption(copyOpts))
		if err != nil {
			return err
		}
		start := time.Now()
		dst := fmt.Sprintf("copy-conc=%d-psize=%d", t.conc, t.psize)
		// fmt.Fprintln(os.Stderr, dst)
		if _, err := fsys.Copy(ctx, dst, src); err != nil {
			return err
		}
		dstSize, err := getSize(ctx, fsys, dst)
		if err != nil {
			return err
		}
		if size != dstSize {
			return errors.New("source and destination size don't match")
		}
		fmt.Printf("%d, %d, %d, %0.2f\n",
			size/gigabyte,
			t.conc,
			t.psize,
			time.Since(start).Seconds())
		if err := fsys.Remove(ctx, dst); err != nil {
			return err
		}
	}
	return nil
}

func createSrcFile(ctx context.Context, bucket string, size int64) (string, error) {
	fsys, err := backend(ctx, bucket)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("copy-source-%d", size/gigabyte)
	if f, err := fsys.OpenFile(ctx, key); err == nil {
		info, err := f.Stat()
		if err != nil {
			return "", err
		}
		f.Close()
		if info.Size() == size {
			return key, nil
		}
	}
	fmt.Fprintln(os.Stderr, "writing source file")
	body := io.LimitReader(rand.Reader, size)
	if _, err := fsys.Write(ctx, key, body); err != nil {
		return "", err
	}
	return key, nil
}

func backend(ctx context.Context, bucket string, opts ...func(*s3.BucketFS)) (*s3.BucketFS, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewBucketFS(s3v2.NewFromConfig(cfg), bucket, opts...), nil
}

func getSize(ctx context.Context, fsys *s3.BucketFS, name string) (int64, error) {
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		return 0, err
	}
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
