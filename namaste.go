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
	NamasteTypeObject = "ocfl_object" // type string for OCFL Object declaration
	NamasteTypeStore  = "ocfl"        // type string for OCFL Storage Root declaration
)

var (
	ErrNamasteNotExist = fmt.Errorf("missing NAMASTE declaration: %w", fs.ErrNotExist)
	ErrNamasteInvalid  = errors.New("invalid NAMASTE declaration contents")
	ErrNamasteMultiple = errors.New("multiple NAMASTE declarations found")
	namasteRE          = regexp.MustCompile(`^0=([a-z_]+)_([0-9]+\.[0-9]+)$`)
)

// Namaste represents a NAMASTE declaration
type Namaste struct {
	Type    string
	Version Spec
}

// FindNamaste returns the NAMASTE declaration from a fs.DirEntry slice. An
// error is returned if the number of declarations is not one.
func FindNamaste(items []fs.DirEntry) (Namaste, error) {
	var found []Namaste
	for _, e := range items {
		if !e.Type().IsRegular() {
			continue
		}
		if dec, err := ParseNamaste(e.Name()); err == nil {
			found = append(found, dec)
		}
	}
	switch len(found) {
	case 0:
		return Namaste{}, ErrNamasteNotExist
	case 1:
		return found[0], nil
	default:
		return Namaste{}, ErrNamasteMultiple
	}
}

// Name returns the filename for d (0=TYPE_VERSION) or an empty string if d is
// empty
func (n Namaste) Name() string {
	if n.Type == "" || n.Version.Empty() {
		return ""
	}
	return "0=" + n.Type + `_` + string(n.Version)
}

// Body returns the expected file contents of the namaste declaration
func (n Namaste) Body() string {
	if n.Type == "" || n.Version.Empty() {
		return ""
	}
	return n.Type + `_` + string(n.Version) + "\n"
}

func ParseNamaste(name string) (n Namaste, err error) {
	m := namasteRE.FindStringSubmatch(name)
	if len(m) != 3 {
		err = ErrNamasteNotExist
		return
	}
	n.Type = m[1]
	n.Version = Spec(m[2])
	return n, nil
}

// ValidateNamaste validates a namaste declaration
func ValidateNamaste(ctx context.Context, fsys FS, name string) error {
	d, err := ParseNamaste(path.Base(name))
	if err != nil {
		return err
	}
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		return fmt.Errorf("opening declaration: %w", err)
	}
	defer f.Close()
	decl, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("reading declaration: %w", err)
	}
	if string(decl) != d.Body() {
		return ErrNamasteInvalid
	}
	return nil
}

func WriteDeclaration(ctx context.Context, root WriteFS, dir string, d Namaste) error {
	cont := strings.NewReader(d.Body())
	_, err := root.Write(ctx, path.Join(dir, d.Name()), cont)
	if err != nil {
		return fmt.Errorf(`writing declaration: %w`, err)
	}
	return nil
}
