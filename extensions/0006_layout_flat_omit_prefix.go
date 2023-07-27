package extensions

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/srerickson/ocfl"
)

const Ext0006 = "0006-flat-omit-prefix-storage-layout"

// Extension 0006-flat-omit-prefix-storage-layout
type LayoutFlatOmitPrefix struct {
	ExtensionName string `json:"extensionName"`
	Delimiter     string `json:"delimiter"`
}

func NewLayoutFlatOmitPrefix() *LayoutFlatOmitPrefix {
	return &LayoutFlatOmitPrefix{
		ExtensionName: Ext0006,
		Delimiter:     ``,
	}
}

// Name returs the registered name
func (l *LayoutFlatOmitPrefix) Name() string {
	return Ext0006
}

func (conf *LayoutFlatOmitPrefix) NewFunc() (LayoutFunc, error) {
	if conf.Delimiter == "" {
		return nil, errors.New("missing required layout configuration 'delimeter'")
	}
	return func(id string) (string, error) {
		dir := id
		lowerID := strings.ToLower(id)
		lowerDelim := strings.ToLower(conf.Delimiter)
		offset := strings.LastIndex(lowerID, lowerDelim)
		if offset > -1 {
			dir = id[offset+len(conf.Delimiter):]
		}
		if dir == ocfl.ExtensionsDir || !fs.ValidPath(dir) {
			return "", fmt.Errorf("object id %q is invalid for layout %q", id, Ext0006)
		}
		return dir, nil
	}, nil
}
