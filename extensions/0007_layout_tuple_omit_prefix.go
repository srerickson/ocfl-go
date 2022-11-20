package extensions

import (
	"fmt"
	"path"
	"strings"
)

const (
	Ext0007  = "0007-n-tuple-omit-prefix-storage-layout"
	padLeft  = "left"
	padRight = "right"
)

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

func NewLayoutTupleOmitPrefix() *LayoutTupleOmitPrefix {
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

func (l LayoutTupleOmitPrefix) NewFunc() (LayoutFunc, error) {
	if l.TupleSize < 1 {
		return nil, fmt.Errorf("invalid tupleSize %d", l.TupleSize)
	}
	if l.TupleNum < 1 {
		return nil, fmt.Errorf("invalid tupleNum %d", l.TupleNum)
	}
	if l.Padding != "left" && l.Padding != "right" {
		return nil, fmt.Errorf("invalid padding %s", l.Padding)
	}
	return l.layout, nil
}

func (l LayoutTupleOmitPrefix) layout(id string) (string, error) {
	size := l.TupleNum * l.TupleSize
	for _, b := range []byte(id) {
		// only asci characters
		if b < 0x20 || b > 0x7F {
			return "", fmt.Errorf("%w:'%s'", ErrInvalidLayoutID, id)
		}
	}
	trimID := id
	prefix := ""
	if idx := strings.LastIndex(id, l.Delimiter); idx > 0 {
		prefix = id[:idx+len(l.Delimiter)]
		if prefix == id {
			return "", fmt.Errorf("invalid id")
		}
		trimID = strings.TrimPrefix(id, prefix)
	}
	if strings.IndexRune(trimID, '/') > 0 {
		return "", fmt.Errorf("%w:'%s'", ErrInvalidLayoutID, id)

	}
	padded := trimID
	if padlen := size - len(padded); padlen > 0 {
		pad := make([]byte, padlen)
		for i := range pad {
			pad[i] = '0'
		}
		if l.Padding == "left" {
			padded = string(pad) + padded
		} else {
			padded = padded + string(pad)
		}
	}
	if l.Reverse {
		rev := []rune(padded)
		for i := 0; i < len(rev)/2; i++ {
			j := len(rev) - i - 1
			rev[i], rev[j] = rev[j], rev[i]
		}
		padded = string(rev)
	}
	tuples := ""
	for i := 0; i < l.TupleNum; i++ {
		tuples = path.Join(tuples, padded[i*l.TupleSize:(i+1)*l.TupleSize])
	}
	return path.Join(tuples, trimID), nil
}
