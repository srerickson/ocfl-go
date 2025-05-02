package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"

	restate "github.com/restatedev/sdk-go"
	"github.com/restatedev/sdk-go/server"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/fs/local"
)

type OCFLObject struct {
	Root *ocfl.Root
}

func (o OCFLObject) GetInventory(ctx restate.ObjectContext, _ restate.Void) (*ocfl.RawInventory, error) {
	rootInventoryKey := "inventory"
	objID := restate.Key(ctx)
	inv, err := restate.Get[*ocfl.RawInventory](ctx, rootInventoryKey)
	if err != nil {
		return nil, err
	}
	if inv != nil {
		return inv, nil
	}
	obj, err := o.Root.NewObject(ctx, objID, ocfl.ObjectMustExist())
	if err != nil {
		return nil, err
	}
	restate.Set(ctx, rootInventoryKey, obj.Inventory())
	return restate.Get[*ocfl.RawInventory](ctx, rootInventoryKey)
}

func main() {
	flag.Parse()
	dir := flag.Arg(0)
	if err := run(dir); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(rootDir string) error {
	ctx := context.Background()
	if rootDir == "" {
		return errors.New("root argument required")
	}
	fsys, err := local.NewFS(rootDir)
	if err != nil {
		return err
	}
	root, err := ocfl.NewRoot(ctx, fsys, ".")
	if err != nil {
		return err
	}
	obj := OCFLObject{
		Root: root,
	}
	return server.NewRestate().
		Bind(restate.Reflect(obj)).
		Start(ctx, "0.0.0.0:9080")
}
