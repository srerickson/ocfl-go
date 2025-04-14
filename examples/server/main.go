package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/srerickson/ocfl-go"
	server "github.com/srerickson/ocfl-go/examples/server/server"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

func main() {
	if err := run(os.Args, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	ctx := context.Background()
	flags := flag.NewFlagSet("", flag.ExitOnError)
	flags.Parse(args[1:])
	rootPath := flags.Arg(0)
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{}))
	if rootPath == "" {
		err := errors.New("missing required argument: OCFL storage root path")
		return err
	}
	logger.Info("using storage root", "path", rootPath)
	fsys := ocflfs.DirFS(rootPath)
	root, err := ocfl.NewRoot(ctx, fsys, ".")
	if err != nil {
		return err
	}
	index := &server.MapRootIndex{}
	if err := index.ReIndex(root.Objects(ctx)); err != nil {
		return err
	}
	templates, err := server.ReadTemplates()
	if err != nil {
		return err
	}
	srv := server.NewServer(logger, root, index, templates)
	return http.ListenAndServe(":8877", srv)
}
