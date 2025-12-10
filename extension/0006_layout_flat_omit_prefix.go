package extension

import (
	"fmt"
	"io/fs"
	"strings"
)

const (
	ext0006   = "0006-flat-omit-prefix-storage-layout"
	delimiter = "delimiter"
)

// LayoutFlatOmitPrefix implements 0006-flat-omit-prefix-storage-layout
type LayoutFlatOmitPrefix struct {
	Base
	Delimiter string `json:"delimiter"`
}

// Ext0006 returns a new instance of 0006-flat-omit-prefix-storage-layout with default values
func Ext0006() Extension {
	return &LayoutFlatOmitPrefix{
		Base:      Base{ExtensionName: ext0006},
		Delimiter: ``,
	}
}

func (l LayoutFlatOmitPrefix) Valid() error {
	if l.Delimiter == "" {
		return fmt.Errorf("required field not set in extension config: %q", delimiter)
	}
	return nil
}

func (l LayoutFlatOmitPrefix) Resolve(id string) (string, error) {
	if err := l.Valid(); err != nil {
		return "", err
	}
	dir := id
	lowerID := strings.ToLower(id)
	lowerDelim := strings.ToLower(l.Delimiter)
	offset := strings.LastIndex(lowerID, lowerDelim)
	if offset > -1 {
		dir = id[offset+len(l.Delimiter):]
	}
	if dir == extensions || !fs.ValidPath(dir) {
		return "", fmt.Errorf("object id %q is invalid for the layout", id)
	}
	return dir, nil
}
