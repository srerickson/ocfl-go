package extension

import "encoding/json"

const ext0002 = "0002-flat-direct-storage-layout"

// LayoutFlatDirect implements 0002-flat-direct-storage-layout
type LayoutFlatDirect struct{}

var _ (Layout) = (*LayoutFlatDirect)(nil)
var _ (Extension) = (*LayoutFlatDirect)(nil)

// Ext0002 returns new instance of 0002-flat-direct-storage-layout with default values
func Ext0002() Extension { return &LayoutFlatDirect{} }

func (l LayoutFlatDirect) Name() string                      { return ext0002 }
func (l LayoutFlatDirect) Resolve(id string) (string, error) { return id, nil }

func (l LayoutFlatDirect) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"extensionName": ext0002})
}
