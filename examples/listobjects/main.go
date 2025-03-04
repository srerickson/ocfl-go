package main

// logs ids and paths for all objects in a storage root
import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	awsS3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/logging"
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
	if err := listObjects(ctx, storeConn, numgos, logger); err != nil {
		logger.Error("exit with errors", "err", err)
		os.Exit(1)
	}
}

func listObjects(ctx context.Context, storeConn string, numgos int, log *slog.Logger) (err error) {
	fsys, dir, err := parseStoreConn(ctx, storeConn)
	if err != nil {
		return fmt.Errorf("can't parse storage root argument: %w", err)
	}
	root, err := ocfl.NewRoot(ctx, fsys, dir)
	if err != nil {
		return nil
	}
	for obj, err := range root.ObjectsBatch(ctx, numgos) {
		if err != nil {
			log.Error(err.Error())
			continue
		}
		fmt.Println(obj.Inventory().ID())
	}
	return nil
}

func parseStoreConn(ctx context.Context, name string) (ocflfs.FS, string, error) {
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
		return ocflfs.DirFS(name), ".", nil
	}
}
