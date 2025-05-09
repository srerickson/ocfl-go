package ocfl

import (
	"context"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

type CachedInventory struct {
	*Inventory
	Digest string
}

type InventoryCache interface {
	GetInventory(ctx context.Context, fsys ocflfs.FS, dir string) (*CachedInventory, error)
	SetInventory(ctx context.Context, fsys ocflfs.FS, dir string, inv *Inventory) error
	//UnsetInventory(ctx context.Context, fsys ocflfs.FS, dir string) error
}
