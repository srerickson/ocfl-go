package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
	"github.com/srerickson/ocfl/validation"
	"golang.org/x/exp/slog"
)

var typ string

func main() {
	flag.StringVar(&typ, "t", "object", "resource type ('object' or 'store')")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	flag.Parse()
	root := flag.Arg(0)
	if root == "" {
		err := errors.New("missing required argument: path to object root")
		logger.Error(err.Error())
		os.Exit(1)
	}
	if typ != "object" && typ != "store" {
		err := fmt.Errorf("uknown resource type '%s'", typ)
		logger.Error(err.Error())
		os.Exit(1)
	}
	fsys := ocfl.NewFS(os.DirFS(root))
	ctx := context.Background()
	var result *validation.Result
	switch typ {
	case "object":
		_, result = ocflv1.ValidateObject(ctx, fsys, ".", ocflv1.ValidationLogger(logger))
	case "store":
		result = ocflv1.ValidateStore(ctx, fsys, ".", ocflv1.ValidationLogger(logger))
	}
	if err := result.Err(); err != nil {
		logger.Error("not a valid OCFL object", "err", err)
		os.Exit(1)
	}
}
