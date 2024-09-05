package testutil

import (
	"encoding/json"
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
		Prefix: "",
	}
}

// CustomLayout implements extension.Layout
type CustomLayout struct {
	Prefix string `json:"prefix"`
}

var _ extension.Layout = (*CustomLayout)(nil)

func (l CustomLayout) Resolve(id string) (string, error) {
	return path.Join(l.Prefix, url.QueryEscape(id)), nil
}

func (l CustomLayout) Valid() error {
	return nil
}

func (l CustomLayout) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"extensionName": customExtensionName,
		"prefix":        l.Prefix,
	})
}

func (l CustomLayout) Name() string { return customExtensionName }
