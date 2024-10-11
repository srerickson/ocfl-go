package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/local"
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/ocflv1"
)

var (
	objPath string // path to object
	srcDir  string // path to content directory
	msg     string // message for new version
	algID   string // digest algorith (sha512 or sha256)
	newID   string // ID for new object
	user    ocfl.User

	logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{}))
)

func main() {
	ctx := context.Background()
	ocflv1.Enable()
	flag.StringVar(&srcDir, "obj", "", "directory of object to commit to")
	flag.StringVar(&srcDir, "src", "", "directory with new version content")
	flag.StringVar(&msg, "msg", "", "message field for new version")
	flag.StringVar(&user.Name, "name", "", "name field for new version")
	flag.StringVar(&user.Address, "email", "", "email field for new version")
	flag.StringVar(&algID, "alg", "sha512", "digest algorith for new version")
	flag.StringVar(&newID, "id", "", "object ID (required for new objects)")
	flag.Parse()

	var missing []string
	flag.VisitAll(func(f *flag.Flag) {
		if f.Name == "id" || f.Name == "alg" {
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
	writeFS, err := local.NewFS(objPath)
	if err != nil {
		quit(err)
	}
	obj, err := ocfl.NewObject(ctx, writeFS, ".")
	if err != nil {
		quit(err)
	}
	if !obj.Exists() && newID == "" {
		err := errors.New("object needs to be created, but 'id' flag is missing")
		quit(err)
	}
	alg, err := digest.DefaultRegister().Get(algID)
	if err != nil {
		quit(err)
	}
	stage, err := ocfl.StageDir(ctx, ocfl.DirFS(srcDir), ".", alg)
	if err != nil {
		quit(err)
	}
	err = obj.Commit(ctx, &ocfl.Commit{
		ID:      newID,
		Stage:   stage,
		Message: msg,
		User:    user,
	})
	if err != nil {
		quit(err)
	}
}

func quit(err error) {
	logger.Error(err.Error())
	os.Exit(1)
}
