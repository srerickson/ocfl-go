package testutil

import (
	"net/url"
	"path"

	"github.com/srerickson/ocfl-go/extension"
)

const (
	customExtensionName = "NNNN-custom-extension"
	prefix              = "prefix"
)

func NewCustomLayout() extension.Extension {
	return &CustomLayout{
		Base:   extension.Base{ExtensionName: customExtensionName},
		Prefix: "",
	}
}

// CustomLayout implements extension.Layout
type CustomLayout struct {
	extension.Base
	Prefix string `json:"prefix"`
}

var _ extension.Layout = (*CustomLayout)(nil)

func (l CustomLayout) Name() string { return customExtensionName }
func (l CustomLayout) Doc() string  { return "" }

func (l CustomLayout) Resolve(id string) (string, error) {
	return path.Join(l.Prefix, url.QueryEscape(id)), nil
}

func (l CustomLayout) Valid() error {
	return nil
}
