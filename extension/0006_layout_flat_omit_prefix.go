package extension

import (
	"errors"
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
		return errors.New("required field not set: " + delimiter)
	}
	return nil
}

func (l LayoutFlatOmitPrefix) Resolve(id string) (string, error) {
	if l.Delimiter == "" {
		return "", errors.New("missing required layout configuration: " + delimiter)
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
