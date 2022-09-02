package extensions

import "errors"

const Ext0007 = "0007-n-tuple-omit-prefix-storage-layout"

// Extension 0007-n-tuple-omit-prefix-storage-layout
type LayoutTupleOmitPrefix struct {
	ExtensionName string `json:"extensionName"`
	Delimiter     string `json:"delimiter"`
	TupleSize     int    `json:"tupleSize"`
	TupleNum      int    `json:"numberOfTuples"`
	Padding       string `json:"zeroPadding"`
	Reverse       bool   `json:"reverseObjectRoot"`
}

var _ Layout = (*LayoutTupleOmitPrefix)(nil)
var _ Extension = (*LayoutTupleOmitPrefix)(nil)

func NewLayoutTupleOmitPrefix() Extension {
	return &LayoutTupleOmitPrefix{
		ExtensionName: Ext0007,
		Delimiter:     `:`,
		TupleSize:     3,
		TupleNum:      3,
		Padding:       "left",
		Reverse:       false,
	}
}

// Name returs the registered name
func (l *LayoutTupleOmitPrefix) Name() string {
	return Ext0007
}

func (conf *LayoutTupleOmitPrefix) NewFunc() (LayoutFunc, error) {
	return nil, errors.New("not implemented")
}
