package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/srerickson/ocfl-go"
	server "github.com/srerickson/ocfl-go/examples/server/server"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

func main() {
	ctx := context.Background()
	flag.Parse()
	rootPath := flag.Arg(0)
	if err := startServer(ctx, rootPath); err != nil {
		log.Fatal(err)
	}
}

func startServer(ctx context.Context, rootPath string) error {
	fsys := ocflfs.DirFS(rootPath)
	root, err := ocfl.NewRoot(ctx, fsys, ".")
	if err != nil {
		return err
	}
	index := &server.MapRootIndex{}
	if err := index.ReIndex(root.Objects(ctx)); err != nil {
		return err
	}
	srv, err := server.NewServer(root, index)
	if err != nil {
		return err
	}
	return http.ListenAndServe(":8877", srv)
}
