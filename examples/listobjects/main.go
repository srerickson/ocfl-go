package main

// lists IDs for all objects in a storage root

import (
	"context"
	"flag"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/cloud"
	"github.com/srerickson/ocfl/logging"
	"github.com/srerickson/ocfl/ocflv1"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
)

var numgos int

func main() {
	ctx := context.Background()
	logger := logging.DefaultLogger()
	flag.IntVar(&numgos, "gos", runtime.NumCPU(), "number of goroutines used for inventory downloading")
	flag.Parse()
	storeURI := flag.Arg(0)
	if storeURI == "" {
		logger.Error("missing required storage root URI")
		os.Exit(1)
	}
	fsys, dir, err := parseURI(ctx, storeURI)
	if err != nil {
		logger.Error("can't parse storage root URI", "err", err)
		os.Exit(1)
	}
	if err := listObjects(ctx, fsys, dir, numgos, logger); err != nil {
		logger.Error("exit with errors", "err", err)
		os.Exit(1)
	}
}

func listObjects(ctx context.Context, fsys ocfl.FS, dir string, gos int, logger *slog.Logger) error {
	// channel for passing object roots from iterator to concurrent inventory
	// download workers
	objRootChan := make(chan *ocfl.ObjectRoot)
	defer close(objRootChan)

	// errgroup for concurrent inventory downloads
	group, ctx := errgroup.WithContext(ctx)
	for i := 0; i < gos; i++ {
		group.Go(func() error {
			for objRoot := range objRootChan {
				if err := ctx.Err(); err != nil {
					return err
				}
				obj := ocflv1.Object{ObjectRoot: *objRoot}
				if err := obj.SyncInventory(ctx); err != nil {
					logger.Error("invalid object", "obj", *objRoot, "err", err)
					return err
				}
				logger.Info(obj.Inventory.ID)
			}
			return nil
		})

	}
	return ocfl.ObjectRoots(ctx, fsys, ocfl.Dir(dir), func(obj *ocfl.ObjectRoot) error {
		objRootChan <- obj
		return nil
	})
}

func parseURI(ctx context.Context, name string) (ocfl.FS, string, error) {
	//if we were using cloud-based backend:
	rl, err := url.Parse(name)
	if err != nil {
		return nil, "", err
	}
	switch rl.Scheme {
	case "azblob":
		bucket, err := blob.OpenBucket(ctx, rl.Scheme+"://"+rl.Host)
		if err != nil {
			return nil, "", err
		}
		return cloud.NewFS(bucket), strings.TrimPrefix(rl.Path, "/"), nil
	default:
		return ocfl.DirFS(name), ".", nil
	}
	// return ocfl.DirFS(name), ".", nil
}
