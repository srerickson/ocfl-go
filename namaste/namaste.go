package namaste

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/srerickson/ocfl/backend"
	spec "github.com/srerickson/ocfl/spec"
)

const (
	ObjectType = "ocfl_object"
	StoreType  = "ocfl"
)

var (
	ErrNotExist = errors.New("NAMASTE declaration not found")
	ErrMultiple = errors.New("multiple NAMASTE declarations found")
	ErrOpen     = errors.New("could not open NAMASTE declaration")
	ErrWrite    = errors.New("could not write NAMASTE declaration")
	ErrContents = errors.New("invalid NAMASTE decleration contents")
	namasteRE   = regexp.MustCompile(`^0=([a-z_]+)_([0-9]+\.[0-9]+)$`)
)

type Declaration struct {
	Type    string
	Version spec.Num
}

// FindDeclaration returns the declaration from a slice of fs.DirEntrys. An
// error is returned if the number of declarations is is not one.
func FindDelcaration(items []fs.DirEntry) (Declaration, error) {
	var found []Declaration
	for _, e := range items {
		if !e.Type().IsRegular() {
			continue
		}
		dec := Declaration{}
		if err := ParseName(e.Name(), &dec); err != nil {
			continue
		}
		found = append(found, dec)
	}
	switch len(found) {
	case 0:
		return Declaration{}, ErrNotExist
	case 1:
		return found[0], nil
	}
	return Declaration{}, ErrMultiple
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

func ParseName(name string, dec *Declaration) error {
	m := namasteRE.FindStringSubmatch(name)
	if len(m) != 3 {
		return ErrNotExist
	}
	dec.Type = m[1]
	err := spec.Parse(m[2], &dec.Version)
	if err != nil {
		return ErrNotExist
	}
	return nil
}

// Validate validates a namaste declaration with path name
func Validate(ctx context.Context, root fs.FS, name string) error {
	var d Declaration
	if err := ParseName(path.Base(name), &d); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := root.Open(name)
	if err != nil {
		return fmt.Errorf(`%w: %s`, ErrOpen, err.Error())
	}
	defer f.Close()
	decl, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if string(decl) != d.Contents() {
		return ErrContents
	}
	return nil
}

func (d Declaration) Write(root backend.Writer, dir string) error {
	cont := strings.NewReader(d.Contents())
	_, err := root.Write(path.Join(dir, d.Name()), cont)
	if err != nil {
		return fmt.Errorf(`%w: %s`, ErrWrite, err.Error())
	}
	return nil
}
