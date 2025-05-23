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

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

const (
	NamasteTypeObject = "ocfl_object" // type string for OCFL Object declaration
	NamasteTypeRoot   = "ocfl"        // type string for OCFL Storage Root declaration
)

var (
	ErrNamasteNotExist = fmt.Errorf("missing NAMASTE declaration: %w", fs.ErrNotExist)
	ErrNamasteContents = errors.New("invalid NAMASTE declaration contents")
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
		if e.IsDir() {
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

// Name returns the filename for n ('0=TYPE_VERSION') or an empty string if n is
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

// IsObject returs true if n's type is 'ocfl_object'
func (n Namaste) IsObject() bool {
	return n.Type == NamasteTypeObject
}

// IsRoot returns true if n's type is 'ocfl'
func (n Namaste) IsRoot() bool {
	return n.Type == NamasteTypeRoot
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
func ValidateNamaste(ctx context.Context, fsys ocflfs.FS, name string) (err error) {
	nam, err := ParseNamaste(path.Base(name))
	if err != nil {
		return
	}
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("opening %q: %w", name, ErrNamasteNotExist)
			return
		}
		err = fmt.Errorf("opening %q: %w", name, err)
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	decl, err := io.ReadAll(f)
	if err != nil {
		err = fmt.Errorf("reading %q: %w", name, err)
		return
	}
	if string(decl) != nam.Body() {
		err = fmt.Errorf("contents of %q: %w", name, ErrNamasteContents)
		return
	}
	return
}

func WriteDeclaration(ctx context.Context, root ocflfs.FS, dir string, d Namaste) error {
	cont := strings.NewReader(d.Body())
	_, err := ocflfs.Write(ctx, root, path.Join(dir, d.Name()), cont)
	if err != nil {
		return fmt.Errorf(`writing declaration: %w`, err)
	}
	return nil
}
