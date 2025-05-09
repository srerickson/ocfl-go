package ocfl

import (
	"context"
)

// InventoryCache is used to cache inventories and optimize initialization of
// new Objects.
type InventoryCache interface {
	// GetInventory returns a *CachedInventory with the inventory and JSON
	// digest of the inventory previous set for the key. If the inventory
	// doesn't exist in the cache both return values are nil.
	GetInventory(ctx context.Context, key string) (*CachedInventory, error)
	// SetInventory adds inv to the cache, associating it with key. If
	// inv.Digest() is an empty string, SetInventory must return an error.
	SetInventory(ctx context.Context, key string, inv *Inventory) error
	//UnsetInventory(ctx context.Context, fsys ocflfs.FS, dir string) error
}

// CachedInventory is the value returned from an InventoryCache. It includes
// the original inventories digest value to track the expected value of the
// inventory's sidevar file.
type CachedInventory struct {
	*Inventory
	// Digest is the digest of the inventory.json that the Inventory was read
	// from.
	Digest string
}
