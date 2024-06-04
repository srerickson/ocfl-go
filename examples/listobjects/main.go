package main

// logs ids and paths for all objects in a storage root
import (
	"context"
	"flag"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	awsS3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/s3"
	"github.com/srerickson/ocfl-go/internal/pipeline"
	"github.com/srerickson/ocfl-go/logging"
	"github.com/srerickson/ocfl-go/ocflv1"
)

var numgos int

func main() {
	ctx := context.Background()
	// logging.SetDefaultLevel(slog.LevelDebug)
	logger := logging.DefaultLogger()
	flag.IntVar(&numgos, "gos", runtime.NumCPU(), "number of goroutines used for inventory downloading")
	flag.Parse()
	storeConn := flag.Arg(0)
	if storeConn == "" {
		logger.Error("missing required storage root URI")
		os.Exit(1)
	}
	fsys, dir, err := parseStoreConn(ctx, storeConn)
	if err != nil {
		logger.Error("can't parse storage root argument", "err", err)
		os.Exit(1)
	}
	if err := listObjects(ctx, fsys, dir, numgos, logger); err != nil {
		logger.Error("exit with errors", "err", err)
		os.Exit(1)
	}
}

func listObjects(ctx context.Context, fsys ocfl.FS, dir string, gos int, logger *slog.Logger) error {
	objectRoots := func(yield func(*ocfl.ObjectRoot) bool) {
		ocfl.ObjectRoots(ctx, fsys, dir)(func(obj *ocfl.ObjectRoot, err error) bool {
			if err != nil {
				return false
			}
			return yield(obj)
		})
	}
	readInventories := func(root *ocfl.ObjectRoot) (*ocflv1.Object, error) {
		obj := &ocflv1.Object{
			ObjectRoot: *root,
		}
		if err := obj.SyncInventory(ctx); err != nil {
			return nil, err
		}
		return obj, nil
	}
	var err error
	resultIter := pipeline.Results(objectRoots, readInventories, gos)
	resultIter(func(r pipeline.Result[*ocfl.ObjectRoot, *ocflv1.Object]) bool {
		if r.Err != nil {
			err = r.Err
			return false
		}
		logger.Info("found object", "id", r.Out.Inventory.ID, "path", r.In.Path)
		return true
	})
	return err
}

func parseStoreConn(ctx context.Context, name string) (ocfl.FS, string, error) {
	//if we were using s3-based backend:
	rl, err := url.Parse(name)
	if err != nil {
		return nil, "", err
	}
	switch rl.Scheme {
	case "s3":
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, "", err
		}
		fsys := &s3.BucketFS{
			S3:     awsS3.NewFromConfig(cfg),
			Bucket: rl.Host,
			Logger: logging.DefaultLogger(),
		}
		return fsys, strings.TrimPrefix(rl.Path, "/"), nil
	default:
		return ocfl.DirFS(name), ".", nil
	}
}
