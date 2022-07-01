package extensions

import "fmt"

const Ext0002 = "0002-flat-direct-storage-layout"

type LayoutFlatDirect struct {
	ExtensionName string `json:"extensionName"`
}

var _ Layout = (*LayoutFlatDirect)(nil)
var _ Extension = (*LayoutFlatDirect)(nil)

func NewLayoutFlatDirect() Extension {
	return &LayoutFlatDirect{
		ExtensionName: Ext0002,
	}
}

func (l *LayoutFlatDirect) Name() string {
	return Ext0002
}

func (l *LayoutFlatDirect) NewFunc() (LayoutFunc, error) {
	if l.ExtensionName != l.Name() {
		return nil, fmt.Errorf("%s: unexpected extensionName %s", l.Name(), l.ExtensionName)
	}
	return func(id string) (string, error) {
		return id, nil
	}, nil
}
