package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"os"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/local"
	"github.com/srerickson/ocfl-go/ocflv1"
)

var (
	storeURI string // path to content directory
	srcDir   string // path to content directory
	id       string // object id to commit
	msg      string // message for new version
	alg      string // digest algorith (sha512 or sha256)
	newObj   bool   // flag indicating new object
	user     ocfl.User

	logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{}))
)

func main() {
	ctx := context.Background()
	flag.StringVar(&srcDir, "src", "", "directory with new version content")
	flag.StringVar(&storeURI, "store", "", "path/uri for storage root")
	flag.StringVar(&id, "id", "", "object id to commit")
	flag.StringVar(&msg, "msg", "", "message field for new version")
	flag.StringVar(&user.Name, "name", "", "name field for new version")
	flag.StringVar(&user.Address, "email", "", "email field for new version")
	flag.StringVar(&alg, "alg", "sha512", "digest algorith for new version")
	flag.BoolVar(&newObj, "new", false, "enable creating new objects")
	flag.Parse()

	var missing []string
	flag.VisitAll(func(f *flag.Flag) {
		if f.Name == "new" || f.Name == "alg" {
			return
		}
		if v := f.Value.String(); v == "" {
			missing = append(missing, f.Name)
		}
	})
	if len(missing) > 0 {
		logger.Error("missing required flags", "flags", missing)
		os.Exit(1)
	}
	// open storage root
	writeFS, err := local.NewFS(storeURI)
	if err != nil {
		quit(err)
	}
	store, err := ocflv1.GetStore(ctx, writeFS, ".")
	if err != nil {
		quit(err)
	}
	// get object
	if _, err := store.GetObject(ctx, id); err != nil {
		// not an error if object doesn't exist
		if !errors.Is(err, fs.ErrNotExist) {
			quit(err)
		}
		if !newObj {
			err := errors.New("object must be created but the 'new' flag is not set")
			quit(err)
		}
	}
	if err == nil && newObj {
		err := errors.New("object exists and 'new' flag is set")
		quit(err)
	}
	stage, err := ocfl.StageDir(ctx, ocfl.DirFS(srcDir), ".", alg)
	if err != nil {
		quit(err)
	}
	err = store.Commit(ctx, id, stage,
		ocflv1.WithMessage(msg),
		ocflv1.WithUser(&user))
	if err != nil {
		quit(err)
	}
}

func quit(err error) {
	logger.Error(err.Error())
	os.Exit(1)
}
