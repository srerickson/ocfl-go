package extensions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl/backend"
)

const (
	extensionsDir       = "extensions"
	extensionConfigFile = "config.json"
)

// global register of extensions
var register = map[string]func() Extension{
	Ext0002: NewLayoutFlatDirect,
	Ext0003: NewLayoutHashIDTuple,
	Ext0004: NewLayoutHashTuple,
}

var ErrUnknown = errors.New("unrecognized extension")

type Extension interface {
	Name() string
}

// Layout is the interface for layout extensions
type Layout interface {
	Extension
	NewFunc() (LayoutFunc, error)
}

// LayoutFunc is a function that maps an object id to a path in the storage root
// or returns an error if the id is invalid
type LayoutFunc func(string) (string, error)

func Get(name string) (Extension, error) {
	ext, ok := register[name]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, ErrUnknown)
	}
	return ext(), nil
}

//
func ReadExtension(fsys fs.FS, root, name string) (Extension, error) {
	ext, err := Get(name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, ErrUnknown)
	}
	err = ReadExtensionConfig(fsys, root, ext)
	if err != nil {
		return nil, err
	}
	return ext, nil
}

func ReadExtensionConfig(fsys fs.FS, root string, ext Extension) error {
	confPath := path.Join(root, extensionsDir, ext.Name(), extensionConfigFile)
	f, err := fsys.Open(confPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", ext.Name(), err)
		}
		return nil
	}
	err = json.NewDecoder(f).Decode(&ext)
	if err != nil {
		return err
	}
	return nil
}

func WriteExtensionConfig(fsys backend.Writer, root string, ext Extension) error {
	confPath := path.Join(root, extensionsDir, ext.Name(), extensionConfigFile)
	pipeR, encWriter := io.Pipe()
	errChan := make(chan error, 1)
	go func() {
		defer close(errChan)
		defer encWriter.Close()
		errChan <- json.NewEncoder(encWriter).Encode(ext)
	}()
	_, err := fsys.Write(confPath, pipeR)
	if err != nil {
		return err
	}
	if err := <-errChan; err != nil {
		return fmt.Errorf("write inventory failed: %w", err)
	}
	return nil
}
