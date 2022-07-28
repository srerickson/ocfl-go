package extensions

import "errors"

const Ext0006 = "0006-flat-omit-prefix-storage-layout"

type LayoutFlatOmitPrefix struct {
	ExtensionName string `json:"extensionName"`
	Delimiter     string `json:"delimiter"`
}

var _ Layout = (*LayoutFlatOmitPrefix)(nil)
var _ Extension = (*LayoutFlatOmitPrefix)(nil)

func NewLayoutFlatOmitPrefix() Extension {
	return &LayoutFlatOmitPrefix{
		ExtensionName: Ext0007,
		Delimiter:     ``,
	}
}

// Name returs the registered name
func (l *LayoutFlatOmitPrefix) Name() string {
	return Ext0007
}

func (conf *LayoutFlatOmitPrefix) NewFunc() (LayoutFunc, error) {
	return nil, errors.New("not implemented")
}
