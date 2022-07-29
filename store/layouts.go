package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl/backend"
	"github.com/srerickson/ocfl/extensions"
)

const layoutName = "ocfl_layout.json"

var (
	ErrNotLayout  = errors.New("not a layout extension")
	defaultLayout = extensions.NewLayoutFlatDirect().(extensions.Layout)
)

func DefaultLayout() extensions.Layout {
	return defaultLayout
}

// ReadLayoutFunc reads the layout extension configuration for the extension name
// in the storage root root, returning the layout func, or an error
func ReadLayoutFunc(fsys fs.FS, root string, name string) (extensions.LayoutFunc, error) {
	ext, err := extensions.ReadExtension(fsys, root, name)
	if err != nil {
		return nil, err
	}
	l, ok := ext.(extensions.Layout)
	if !ok {
		return nil, fmt.Errorf("read layout %s: %w", name, ErrNotLayout)
	}
	return l.NewFunc()
}

// ReadLayout reads the `ocfl_layout.json` files in the storage root
// and unmarshals into the value pointed to by layout
func ReadLayout(fsys fs.FS, root string, layout interface{}) error {
	f, err := fsys.Open(path.Join(root, layoutName))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(layout)
}

// WriteLayout marshals the value pointe to by layout and writes the result to
// the `ocfl_layout.json` files in the storage root.
func WriteLayout(fsys backend.Writer, root string, layout interface{}) error {
	b, err := json.Marshal(layout)
	if err != nil {
		return err
	}
	_, err = fsys.Write(path.Join(root, layoutName), bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	return nil
}
