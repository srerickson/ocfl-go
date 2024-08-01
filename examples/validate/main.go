package main

// validates an OCFL object or storage root on the local filesystem.
import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
)

var (
	objPath string
	logger  = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
)

func main() {
	ctx := context.Background()
	ocflv1.Enable() // setup ocflv1
	flag.Parse()
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
	fsys := ocfl.NewFS(os.DirFS(root))
	obj, err := ocfl.NewObject(ctx, fsys, ".")
	if err != nil {
		return err
	}
	result := obj.Validate(ctx, ocfl.ValidationLogger(logger))
	return result.Err()
}
