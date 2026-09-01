package main

// logs ids and paths for all objects in a storage root
import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/fs/config"
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
	storeCnf, err := config.New(ctx, storeConn, config.WithLogger(log))
	if err != nil {
		return fmt.Errorf("can't parse storage root argument: %w", err)
	}
	root, err := ocfl.NewRoot(ctx, storeCnf.FS, storeCnf.Path)
	if err != nil {
		return nil
	}
	for obj, err := range root.ObjectsBatch(ctx, numgos) {
		if err != nil {
			log.Error(err.Error())
			continue
		}
		fmt.Println(obj.ID())
	}
	return nil
}
