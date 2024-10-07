package extension

import (
	_ "embed"
)

const ext0002 = "0002-flat-direct-storage-layout"

//go:embed docs/0002-flat-direct-storage-layout.md
var ext0002doc []byte

// LayoutFlatDirect implements 0002-flat-direct-storage-layout
type LayoutFlatDirect struct {
	Base
}

var _ (Layout) = (*LayoutFlatDirect)(nil)
var _ (Extension) = (*LayoutFlatDirect)(nil)

// Ext0002 returns new instance of 0002-flat-direct-storage-layout with default values
func Ext0002() Extension {
	return &LayoutFlatDirect{
		Base: Base{ExtensionName: ext0002},
	}
}

func (l LayoutFlatDirect) Documentation() []byte             { return ext0002doc }
func (l LayoutFlatDirect) Resolve(id string) (string, error) { return id, nil }
func (l LayoutFlatDirect) Valid() error                      { return nil }
