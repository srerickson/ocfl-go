package extension

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ext0007           = "0007-n-tuple-omit-prefix-storage-layout"
	padLeft           = "left"
	padRight          = "right"
	zeroPadding       = "zeroPadding"
	reverseObjectRoot = "reverseObjectRoot"
)

// LayoutTupleOmitPrefix implements 0007-n-tuple-omit-prefix-storage-layout.
//
// The Delimiter is matched case-insensitively. The part of the ID after the last Delimiter must be a valid directory name:
// it can't be empty, "." or "..", and it can't contain "/". None of the tuples
// taken from it can be "." or ".." either. IDs that break these rules are
// rejected with ErrInvalidLayoutID. This includes IDs that keep a "/" after
// the delimiter, such as ARKs ("ark:/13030/xyz") under the default ":"
// delimiter: use a delimiter that ends at the last "/" for such IDs (for
// example, "/").
type LayoutTupleOmitPrefix struct {
	Base
	Delimiter string `json:"delimiter"`
	TupleSize int    `json:"tupleSize"`
	TupleNum  int    `json:"numberOfTuples"`
	Padding   string `json:"zeroPadding"`
	Reverse   bool   `json:"reverseObjectRoot"`
}

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

func (l LayoutTupleOmitPrefix) Valid() error {
	if l.Delimiter == "" {
		return fmt.Errorf("required field not set in extension config: %q", delimiter)
	}
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
	// The delimiter is case-insensitive. The ID is ASCII, so offsets in the
	// lowercased ID match offsets in id.
	lowerID := strings.ToLower(id)
	lowerDelim := strings.ToLower(l.Delimiter)
	if idx := strings.LastIndex(lowerID, lowerDelim); idx >= 0 {
		trimID = id[idx+len(lowerDelim):]
	}
	if trimID == "" || trimID == "." || trimID == ".." || strings.Contains(trimID, "/") {
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
	segments := make([]string, 0, l.TupleNum+1)
	for i := 0; i < l.TupleNum; i++ {
		tuple := padded[i*l.TupleSize : (i+1)*l.TupleSize]
		if tuple == "." || tuple == ".." {
			return "", fmt.Errorf("%w:'%s'", ErrInvalidLayoutID, id)
		}
		segments = append(segments, tuple)
	}
	segments = append(segments, trimID)
	return strings.Join(segments, "/"), nil
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
