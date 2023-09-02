package main

// validates an OCFL object or storage root on the local filesystem.
import (
	"context"
	"flag"
	"os"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
	"golang.org/x/exp/slog"
)

var (
	isStore bool
	name    string
	logger  = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
)

func main() {
	flag.BoolVar(&isStore, "store", false, "validate path as an OCFL storage root")
	flag.Parse()
	name = flag.Arg(0)
	if name == "" {
		logger.Error("missing required path argument")
		os.Exit(1)
	}
	if isStore {
		logger = logger.With("storage_root", name)
	} else {
		logger = logger.With("object_path", name)
	}
	if err := validate(name, isStore, logger); err != nil {
		os.Exit(1)
	}
}

func validate(root string, isStore bool, logger *slog.Logger) error {
	fsys := ocfl.NewFS(os.DirFS(root))
	ctx := context.Background()
	if isStore {
		result := ocflv1.ValidateStore(ctx, fsys, ".", ocflv1.ValidationLogger(logger))
		return result.Err()
	}
	_, result := ocflv1.ValidateObject(ctx, fsys, ".", ocflv1.ValidationLogger(logger))
	return result.Err()
}
