package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/iand/logfmtr"
	"github.com/srerickson/ocfl/ocflv1"
	"github.com/srerickson/ocfl/validation"
)

func main() {
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
	log = log.WithName(root)
	fsys := os.DirFS(root)
	err := ocflv1.ValidateObject(
		context.Background(),
		fsys, ".",
		&ocflv1.ValidateObjectConf{Log: validation.NewLog(log)},
	)
	if err != nil {
		log.Error(err, "path is not a valid OCFL object")
		os.Exit(1)
	}
	log.Info("OK")
}
