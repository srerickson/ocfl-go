package extension

const ext0002 = "0002-flat-direct-storage-layout"

// Ext0002 returns new instance of 0002-flat-direct-storage-layout with default values
func Ext0002() Extension {
	return &LayoutFlatDirect{
		Base: Base{ExtensionName: ext0002},
	}
}

// LayoutFlatDirect implements 0002-flat-direct-storage-layout
type LayoutFlatDirect struct {
	Base
}

func (l LayoutFlatDirect) Resolve(id string) (string, error) { return id, nil }
func (l LayoutFlatDirect) Valid() error                      { return nil }
