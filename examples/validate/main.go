package main

// validates an OCFL object or storage root on the local filesystem.
import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

var objPath string

func main() {
	ctx := context.Background()
	flag.Parse()
	logger := slog.Default()
	objPath = flag.Arg(0)
	if objPath == "" {
		logger.Error("missing required object root path argument")
		os.Exit(1)
	}
	if err := validateObject(ctx, objPath, logger); err != nil {
		os.Exit(1)
	}
}

func validateObject(ctx context.Context, root string, logger *slog.Logger) error {
	fsys := ocflfs.DirFS(root)
	result := ocfl.ValidateObject(ctx, fsys, ".", ocfl.ValidationLogger(logger))
	return result.Err()
}
