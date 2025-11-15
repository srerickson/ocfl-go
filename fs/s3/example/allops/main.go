package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path"

	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return errors.New("missing bucket argument")
	}
	ctx := context.Background()
	bucket := args[0]
	fsys, err := backend(ctx, bucket)
	if err != nil {
		return err
	}
	dirPrefix := "test-dir"
	key := path.Join(dirPrefix, "test-file")
	buff := mock.RandBytes(15 * 1024 * 1024)
	log.Println("writing to", key)
	_, err = fsys.Write(ctx, key, bytes.NewReader(buff))
	if err != nil {
		return err
	}

	log.Println("opening", key)
	f, err := fsys.OpenFile(ctx, key)
	if err != nil {
		return err
	}
	defer f.Close()

	log.Println("reading", key)
	_, err = io.ReadAll(f)
	if err != nil {
		return err
	}

	log.Println("copying", key)
	_, err = fsys.Copy(ctx, path.Join(dirPrefix, "copy"), key)
	if err != nil {
		return err
	}

	log.Println("reading entries for", dirPrefix)
	for entry, err := range fsys.DirEntries(ctx, dirPrefix) {
		if err != nil {
			return err
		}
		log.Println("... found", entry.Name())
	}

	log.Println("removing", dirPrefix)
	err = fsys.RemoveAll(ctx, dirPrefix)
	if err != nil {
		return err
	}
	return nil
}

func backend(ctx context.Context, bucket string) (*s3.BucketFS, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewBucketFS(s3v2.NewFromConfig(cfg), bucket), nil
}
