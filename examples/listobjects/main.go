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
	"github.com/srerickson/ocfl-go/backend/s3"
	"github.com/srerickson/ocfl-go/logging"
	"github.com/srerickson/ocfl-go/ocflv1"
)

var numgos int

func main() {
	ocflv1.Enable()
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
	if err := listObjects2(ctx, fsys, dir, numgos, logger); err != nil {
		logger.Error("exit with errors", "err", err)
		os.Exit(1)
	}
}

func listObjects2(ctx context.Context, fsys ocfl.FS, dir string, numgos int, log *slog.Logger) (err error) {
	allFiles, walkErrFn := ocfl.WalkFiles(ctx, fsys, dir)
	defer func() {
		err = walkErrFn()
	}()
	decls := allFiles.Filter(func(f *ocfl.FileRef) bool { return f.Namaste().IsObject() })
	for obj, err := range decls.OpenObjectsBatch(ctx, numgos) {
		if err != nil {
			log.Error(err.Error())
			continue
		}
		id := obj.Inventory().ID()
		fmt.Println(id)
	}
	return
}

// func listObjects(ctx context.Context, fsys ocfl.FS, dir string, gos int, _ *slog.Logger) error {
// 	objectDirs := func(yield func(string) bool) {
// 		for dir, err := range ocfl.ObjectPaths(ctx, fsys, dir) {
// 			if err != nil {
// 				break
// 			}
// 			if !yield(dir) {
// 				break
// 			}
// 		}
// 	}
// 	getID := func(dir string) (ocfl.ReadInventory, error) {
// 		obj, err := ocfl.NewObject(ctx, fsys, dir)
// 		if err != nil {
// 			return nil, err
// 		}
// 		return obj.Inventory(), nil
// 	}
// 	var err error
// 	resultIter := pipeline.Results(objectDirs, getID, gos)
// 	resultIter(func(r pipeline.Result[string, ocfl.ReadInventory]) bool {
// 		if r.Err != nil {
// 			err = r.Err
// 			return false
// 		}
// 		fmt.Println(r.In, r.Out.ID())
// 		return true
// 	})
// 	return err
// }

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
