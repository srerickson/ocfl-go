package spec

import "strings"

const (
	invTypePrefix = "https://ocfl.io/"
	invTypeSuffix = "/spec/#inventory"
)

// InventoryType represents an inventory type string
// for example: https://ocfl.io/1.0/spec/#inventory
type InventoryType struct {
	Num
}

func (inv InventoryType) String() string {
	return invTypePrefix + inv.Num.String() + invTypeSuffix
}

func (invT *InventoryType) UnmarshalText(t []byte) error {
	cut := strings.TrimPrefix(string(t), invTypePrefix)
	cut = strings.TrimSuffix(cut, invTypeSuffix)
	return Parse(cut, &invT.Num)
}

func (invT InventoryType) MarshalText() ([]byte, error) {
	return []byte(invT.String()), nil
}
