// program for quickly testing file transfers to storage backend

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/srerickson/ocfl/backend/cloud"
	"github.com/srerickson/ocfl/logger"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
)

var progress bool

func main() {
	log := logger.DefaultLogger()
	logger.SetVerbosity(10)
	flag.BoolVar(&progress, "progress", false, "progress")
	flag.Parse()
	name := flag.Arg(0)
	if name == "" {
		err := errors.New("file name is required")
		log.Error(err, "quitting")
		os.Exit(1)
	}
	name, err := filepath.Abs(name)
	if err != nil {
		log.Error(err, "quitting")
		os.Exit(1)
	}
	base := filepath.Base(name)
	ctx := context.Background()
	bucketName := "ocfl"
	bucket, err := blob.OpenBucket(ctx, "azblob://"+bucketName)
	if err != nil {
		log.Error(err, "quitting")
		os.Exit(1)
	}
	opts := &blob.WriterOptions{
		BufferSize:     32 * 1024 * 1024,
		MaxConcurrency: runtime.GOMAXPROCS(0) + 4,
	}
	wFS := cloud.NewFS(bucket, cloud.WithLogger(log))
	f, err := os.Open(name)
	if err != nil {
		log.Error(err, "quitting")
		os.Exit(1)
	}
	defer f.Close()
	start := time.Now()
	if !progress {
		if _, err := wFS.WriterOptions(opts).Write(ctx, base, f); err != nil {
			log.Error(err, "transfer failed")
			os.Exit(1)
		}
		log.Info("done", "time", time.Since(start).Seconds())
		return
	}
	progress := ProgressWriter{}
	err = progress.Start(func(w io.Writer) error {
		if _, err := wFS.Write(ctx, base, io.TeeReader(f, w)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Error(err, "transfer failed")
		os.Exit(1)
	}
	log.Info("done", "time", time.Since(start).Seconds())
}
