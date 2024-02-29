package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

const (
	DeclObject = "ocfl_object" // type string for OCFL Object declaration
	DeclStore  = "ocfl"        // type string for OCFL Storage Root declaration
)

var (
	ErrDeclNotExist = fmt.Errorf("missing declaration: %w", fs.ErrNotExist)
	ErrDeclInvalid  = errors.New("invalid NAMASTE declaration contents")
	ErrDeclMultiple = errors.New("multiple NAMASTE declarations found")
	namasteRE       = regexp.MustCompile(`^0=([a-z_]+)_([0-9]+\.[0-9]+)$`)
)

// Declaration represents a NAMASTE Declaration
type Declaration struct {
	Type    string
	Version Spec
}

// FindDeclaration returns the declaration from a fs.DirEntry slice. An
// error is returned if the number of declarations is not one.
func FindDeclaration(items []fs.DirEntry) (Declaration, error) {
	var found []Declaration
	for _, e := range items {
		if !e.Type().IsRegular() {
			continue
		}
		dec := Declaration{}
		if err := ParseDeclaration(e.Name(), &dec); err != nil {
			continue
		}
		found = append(found, dec)
	}
	switch len(found) {
	case 0:
		return Declaration{}, ErrDeclNotExist
	case 1:
		return found[0], nil
	}
	return Declaration{}, ErrDeclMultiple
}

// Name returns the filename for d (0=TYPE_VERSION) or an empty string if d is
// empty
func (d Declaration) Name() string {
	if d.Type == "" || d.Version.Empty() {
		return ""
	}
	return "0=" + d.Type + `_` + d.Version.String()
}

// Contents returns the file contents of the declaration or an empty string if d
// is empty
func (d Declaration) Contents() string {
	if d.Type == "" || d.Version.Empty() {
		return ""
	}
	return d.Type + `_` + d.Version.String() + "\n"
}

func ParseDeclaration(name string, dec *Declaration) error {
	m := namasteRE.FindStringSubmatch(name)
	if len(m) != 3 {
		return ErrDeclNotExist
	}
	dec.Type = m[1]
	var err error
	dec.Version, err = ParseSpec(m[2])
	if err != nil {
		return ErrDeclNotExist
	}
	return nil
}

// ReadDeclaration validates a namaste declaration
func ReadDeclaration(ctx context.Context, fsys FS, filePath string) error {
	var d Declaration
	if err := ParseDeclaration(path.Base(filePath), &d); err != nil {
		return err
	}
	f, err := fsys.OpenFile(ctx, filePath)
	if err != nil {
		return fmt.Errorf("opening declaration: %w", err)
	}
	defer f.Close()
	decl, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("reading declaration: %w", err)
	}
	if string(decl) != d.Contents() {
		return ErrDeclInvalid
	}
	return nil
}

func WriteDeclaration(ctx context.Context, root WriteFS, dir string, d Declaration) error {
	cont := strings.NewReader(d.Contents())
	_, err := root.Write(ctx, path.Join(dir, d.Name()), cont)
	if err != nil {
		return fmt.Errorf(`writing declaration: %w`, err)
	}
	return nil
}
