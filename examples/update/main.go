package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/config"
)

type cmdFlags struct {
	objPath string // path to object
	srcDir  string // path to content directory
	msg     string // message for new version
	algID   string // digest algorith (sha512 or sha256)
	newID   string // ID for new object
	user    ocfl.User
}

var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))

func main() {
	ctx := context.Background()
	if err := runUpdate(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runUpdate(ctx context.Context, args []string) error {
	f, err := parseArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	objCnf, err := config.New(ctx, f.objPath, config.WithLogger(logger))
	if err != nil {
		return err
	}
	obj, err := ocfl.NewObject(ctx, objCnf.FS, objCnf.Path, ocfl.ObjectWithID(f.newID))
	if err != nil {
		return fmt.Errorf("%s: %w", objCnf.Path, err)
	}
	if !obj.Exists() && f.newID == "" {
		return errors.New("'id' flag is required for to a create new objects (object does not exist)")
	}
	alg, err := digest.DefaultRegistry().Get(f.algID)
	if err != nil {
		return err
	}
	stage, err := ocfl.StageDir(ctx, ocflfs.DirFS(f.srcDir), ".", alg)
	if err != nil {
		return err
	}
	update, err := obj.NewUpdatePlan(stage, f.msg, f.user, ocfl.UpdateWithLogger(logger))
	if err != nil {
		return err
	}
	applyCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	err = obj.ApplyUpdatePlan(applyCtx, update, stage.ContentSource)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("received interupt: reverting changes...")
			err = update.Revert(ctx, obj.FS(), obj.Path(), stage)
		}
		return err
	}
	return nil
}

func parseArgs(args []string) (*cmdFlags, error) {
	var f cmdFlags
	set := flag.FlagSet{}
	set.StringVar(&f.objPath, "obj", "", "path to ocfl object to create/update")
	set.StringVar(&f.srcDir, "src", "", "local path with new object content")
	set.StringVar(&f.msg, "msg", "", "message field for new version")
	set.StringVar(&f.user.Name, "name", "", "name field for new version")
	set.StringVar(&f.user.Address, "email", "", "email field for new version")
	set.StringVar(&f.algID, "alg", "sha512", "digest algorith for new version")
	set.StringVar(&f.newID, "id", "", "object ID (required for creating new objects)")
	if err := set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			set.PrintDefaults()
		}
		return nil, err
	}
	var missing []string
	set.VisitAll(func(f *flag.Flag) {
		if f.Name == "id" || f.Name == "alg" {
			return
		}
		if v := f.Value.String(); v == "" {
			missing = append(missing, f.Name)
		}
	})
	if len(missing) > 0 {
		return nil, errors.New("missing required flags: " + strings.Join(missing, ", "))
	}
	return &f, nil
}
