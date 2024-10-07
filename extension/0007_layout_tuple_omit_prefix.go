package extension

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

const (
	ext0007           = "0007-n-tuple-omit-prefix-storage-layout"
	padLeft           = "left"
	padRight          = "right"
	zeroPadding       = "zeroPadding"
	reverseObjectRoot = "reverseObjectRoot"
)

//go:embed docs/0007-n-tuple-omit-prefix-storage-layout.md
var ext0007doc []byte

// LayoutTupleOmitPrefix implements 0007-n-tuple-omit-prefix-storage-layout
type LayoutTupleOmitPrefix struct {
	Base
	Delimiter string `json:"delimiter"`
	TupleSize int    `json:"tupleSize"`
	TupleNum  int    `json:"numberOfTuples"`
	Padding   string `json:"zeroPadding"`
	Reverse   bool   `json:"reverseObjectRoot"`
}

var _ (Layout) = (*LayoutTupleOmitPrefix)(nil)
var _ (Extension) = (*LayoutTupleOmitPrefix)(nil)

// Ext0007 returns a new instance of 0007-n-tuple-omit-prefix-storage-layout with default values
func Ext0007() Extension {
	return &LayoutTupleOmitPrefix{
		Base:      Base{ExtensionName: ext0007},
		Delimiter: `:`,
		TupleSize: 3,
		TupleNum:  3,
		Padding:   padLeft,
		Reverse:   false,
	}
}

func (l LayoutTupleOmitPrefix) Documentation() []byte { return ext0007doc }

func (l LayoutTupleOmitPrefix) Valid() error {
	if l.TupleSize < 1 {
		return fmt.Errorf("invalid %s: %d", tupleSize, l.TupleSize)
	}
	if l.TupleNum < 1 {
		return fmt.Errorf("invalid %s: %d", numberOfTuples, l.TupleNum)
	}
	if l.Padding != padLeft && l.Padding != padRight {
		return fmt.Errorf("invalid padding: %s", l.Padding)
	}
	return nil
}

func (l LayoutTupleOmitPrefix) Resolve(id string) (string, error) {
	if err := l.Valid(); err != nil {
		return "", err
	}
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

func (l LayoutTupleOmitPrefix) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		extensionName:     ext0007,
		delimiter:         l.Delimiter,
		tupleSize:         l.TupleSize,
		numberOfTuples:    l.TupleNum,
		zeroPadding:       l.Padding,
		reverseObjectRoot: l.Reverse,
	})
}
