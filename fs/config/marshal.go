package config

import (
	"context"
	"encoding"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"strings"
)

var _ encoding.TextMarshaler = FSConfig{}
var _ encoding.TextUnmarshaler = (*FSConfig)(nil)

// MarshalText implements [encoding.TextMarshaler] for c: it returns the
// configuration string that [New] would use to build c. Because
// [encoding/json] uses the text interfaces, an FSConfig can be a field in a
// JSON-encoded configuration struct.
//
// Marshaling is delegated to c's FS, which must implement
// [encoding.TextMarshaler]: the backends in this module do, and a backend from
// outside it round-trips by doing the same. Path, when it is not ".", is added
// to the resulting url's path.
func (c FSConfig) MarshalText() ([]byte, error) {
	if c.FS == nil {
		return nil, errors.New("marshaling fs configuration: no file system")
	}
	marshaler, ok := c.FS.(encoding.TextMarshaler)
	if !ok {
		return nil, fmt.Errorf("marshaling fs configuration: %T does not implement encoding.TextMarshaler", c.FS)
	}
	text, err := marshaler.MarshalText()
	if err != nil {
		return nil, fmt.Errorf("marshaling fs configuration: %w", err)
	}
	if c.Path == "" || c.Path == "." {
		return text, nil
	}
	if !fs.ValidPath(c.Path) {
		return nil, fmt.Errorf("marshaling fs configuration: invalid path %q", c.Path)
	}
	u, err := url.Parse(string(text))
	if err != nil {
		return nil, fmt.Errorf("marshaling fs configuration: %T returned an invalid url: %w", c.FS, err)
	}
	u.Path = "/" + strings.TrimPrefix(path.Join(u.Path, c.Path), "/")
	return []byte(u.String()), nil
}

// UnmarshalText implements [encoding.TextUnmarshaler] for c: it replaces c
// with the FSConfig described by text, constructing the backend in the
// process.
//
// The interface leaves no room for a context or for options, so this uses
// [context.Background] and no options. Callers that need either -- to cancel
// the AWS configuration load behind an "s3" string, or to supply their own
// client -- should call [New] instead.
func (c *FSConfig) UnmarshalText(text []byte) error {
	cnf, err := New(context.Background(), string(text))
	if err != nil {
		return err
	}
	*c = *cnf
	return nil
}
