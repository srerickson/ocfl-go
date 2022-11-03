package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/iand/logfmtr"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
	"github.com/srerickson/ocfl/validation"
)

var typ string

func main() {
	flag.StringVar(&typ, "t", "object", "resource type ('object' or 'store')")
	log := logfmtr.NewWithOptions(logfmtr.Options{
		Writer:    os.Stderr,
		Colorize:  true,
		Humanize:  true,
		NameDelim: "/",
	})
	flag.Parse()
	root := flag.Arg(0)
	if root == "" {
		err := errors.New("missing required argument: path to object root")
		log.Error(err, "cannot continue")
		os.Exit(1)
	}
	if typ != "object" && typ != "store" {
		err := fmt.Errorf("uknown resource type '%s'", typ)
		log.Error(err, "cannot continue")
		os.Exit(1)
	}
	log = log.WithName(root)
	fsys := ocfl.NewFS(os.DirFS(root))
	ctx := context.Background()
	var result *validation.Result
	switch typ {
	case "object":
		_, result = ocflv1.ValidateObject(ctx, fsys, ".", ocflv1.ValidationLogger(log))
	case "store":
		result = ocflv1.ValidateStore(ctx, fsys, ".", ocflv1.ValidationLogger(log))
	}
	if err := result.Err(); err != nil {
		log.Error(err, "path is not a valid OCFL object")
		os.Exit(1)
	}
	log.Info("OK")
}
