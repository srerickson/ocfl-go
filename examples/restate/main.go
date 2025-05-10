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

// func (o OCFLObject) Update(ctx restate.ObjectContext, new ocfl.Inventory) error {

// }

func (o OCFLObject) GetState(ctx restate.ObjectContext, v int) (ocfl.DigestMap, error) {
	cache := newCache(ctx)
	objID := restate.Key(ctx)
	obj, err := o.Root.NewObject(
		ctx, objID,
		ocfl.ObjectMustExist(),
		ocfl.ObjectWithInventoryCache(cache),
	)
	if err != nil {
		return nil, err
	}
	ver := obj.Version(v)
	if ver == nil {
		return nil, errors.New("not found")
	}
	return ver.State(), nil
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

func newCache(ctx restate.ObjectContext) *invCache {
	return &invCache{ctx: ctx}
}

type invCache struct {
	ctx restate.ObjectContext
}

func (c invCache) GetInventory(ctx context.Context, _ string) (*ocfl.CachedInventory, error) {
	objCtx, notRestate := ctx.(restate.ObjectContext)
	if !notRestate {
		return nil, errors.New("expected restate object context")
	}
	return restate.Get[*ocfl.CachedInventory](objCtx, "inventory")
}
func (c invCache) SetInventory(ctx context.Context, _ string, inv *ocfl.Inventory) error {
	objCtx, notRestate := ctx.(restate.ObjectContext)
	if !notRestate {
		return errors.New("expected restate object context")
	}
	restate.Set(objCtx, "inventory", &ocfl.CachedInventory{
		Inventory: inv,
		Digest:    inv.Digest(),
	})
	return nil
}
