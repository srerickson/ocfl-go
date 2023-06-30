package main

// lists IDs for all objects in a storage root

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
	"golang.org/x/exp/slog"
)

func main() {
	flag.Parse()
	root := flag.Arg(0)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	if root == "" {
		logger.Error("missing required path argument")
		os.Exit(1)
	}
	if err := list(root, logger); err != nil {
		logger.Error("exit with errors", "err", err)
		os.Exit(1)
	}
}

func list(name string, logger *slog.Logger) error {
	ctx := context.Background()
	fsys := ocfl.NewFS(os.DirFS(name))
	fn := func(obj *ocflv1.Object) error {
		if err := obj.SyncInventory(ctx); err != nil {
			logger.Error("invalid object", "path", obj.Path, "err", err)
			return nil
		}
		fmt.Println(obj.Inventory.ID)
		return nil
	}
	opts := &ocflv1.ScanObjectsOpts{}
	return ocflv1.ScanObjects(ctx, fsys, ".", fn, opts)
}
