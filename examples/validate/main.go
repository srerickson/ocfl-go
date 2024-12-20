package main

// validates an OCFL object or storage root on the local filesystem.
import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/charmbracelet/log"
	"github.com/srerickson/ocfl-go"
)

var objPath string

func main() {
	ctx := context.Background()
	flag.Parse()
	handl := log.New(os.Stderr)
	handl.SetLevel(log.WarnLevel)
	logger := slog.New(handl)
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
	result := ocfl.ValidateObject(ctx, fsys, ".", ocfl.ValidationLogger(logger))
	return result.Err()
}
