package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"net/http"

	"github.com/srerickson/ocfl-go"
	server "github.com/srerickson/ocfl-go/examples/server/server"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

func main() {
	ctx := context.Background()
	flag.Parse()
	rootPath := flag.Arg(0)
	if err := serveOCFL(ctx, rootPath); err != nil {
		log.Fatal(err)
	}
}

func serveOCFL(ctx context.Context, rootPath string) error {
	logger := slog.Default()
	fsys := ocflfs.DirFS(rootPath)
	root, err := ocfl.NewRoot(ctx, fsys, ".")
	if err != nil {
		return err
	}
	index := &server.MapRootIndex{}
	if err := index.ReIndex(root.Objects(ctx)); err != nil {
		return err
	}

	srv := server.NewServer(logger, root, index, indexView, objectView)

	return http.ListenAndServe(":8877", srv)
}
