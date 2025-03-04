package ocfl

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

const (
	invTypePrefix = "https://ocfl.io/"
	invTypeSuffix = "/spec/#inventory"
	specsDir      = "specs"
)

var (
	ErrSpecInvalid  = errors.New("invalid OCFL spec version")
	ErrSpecNotFound = errors.New("OCFL spec file not found")

	// matcher for OCFL specification version format:
	// matches "1.0", "2.1", "2.2-draft"
	verNumRegex = regexp.MustCompile(`^\d\.\d+(\-\w+)?$`)
)

//go:embed specs/*
var specFS embed.FS

// Spec represent an OCFL specification number
type Spec string

func (s Spec) Valid() error {
	if !verNumRegex.MatchString(string(s)) {
		return ErrSpecInvalid
	}
	return nil
}

// Cmp compares Spec v1 to another v2.
// - If v1 is less than v2, returns -1.
// - If v1 is the same as v2, returns 0
// - If v1 is greater than v2, returns 1
// - any valid spec is greater than an invalid spec.
// - if both specs are invlid, Cmp panics.
func (v1 Spec) Cmp(v2 Spec) int {
	f1, suf1, err1 := v1.parse()
	f2, suf2, err2 := v2.parse()
	// handle errors
	if err1 != nil || err2 != nil {
		if err1 == nil {
			return 1 // v1 is larger because v2 is invalid
		}
		if err2 == nil {
			return -1 // v2 is larger because v1 is invalid
		}
		// both are invalid
		panic(errors.Join(err1, err2))
	}
	switch {
	case f1 == f2:
		// if v1 and v2 are numerically equal and one has a suffix, the one
		// with the suffix is *less* than the one without.
		if suf1 == "" && suf2 != "" {
			return 1
		}
		if suf2 == "" && suf1 != "" {
			return -1
		}
		return 0
	case f1 > f2:
		return 1
	default:
		return -1
	}
}

func (s Spec) Empty() bool {
	return s == Spec("")
}

func (s Spec) parse() (float64, string, error) {
	// allow format like 1.2-draft
	if err := s.Valid(); err != nil {
		return 0, "", err
	}
	numStr, suffix, _ := strings.Cut(string(s), "-")
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, "", ErrSpecInvalid
	}
	return val, suffix, nil
}

// InventoryType returns n as an InventoryType
func (s Spec) InventoryType() InventoryType {
	return InventoryType{Spec: s}
}

func WriteSpecFile(ctx context.Context, fsys ocflfs.WriteFS, dir string, n Spec) (string, error) {
	if err := n.Valid(); err != nil {
		return "", err
	}
	glob := specsDir + "/" + "ocfl_" + string(n) + ".*"
	files, err := fs.Glob(specFS, glob)
	if err != nil || len(files) != 1 {
		return "", ErrSpecNotFound
	}
	name := path.Base(files[0])
	dst := path.Join(dir, name)
	if f, err := fsys.OpenFile(ctx, dst); err == nil {
		defer f.Close()
		return "", fmt.Errorf("already exists: %s", dst)
	}
	f, err := specFS.Open(files[0])
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = fsys.Write(ctx, dst, f)
	if err != nil {
		return "", fmt.Errorf("writing OCFL spec file '%s': %w", dst, err)
	}
	return dst, nil
}

// InventoryType represents an inventory type string
// for example: https://ocfl.io/1.0/spec/#inventory
type InventoryType struct {
	Spec
}

func (inv InventoryType) String() string {
	return invTypePrefix + string(inv.Spec) + invTypeSuffix
}

func (invT *InventoryType) UnmarshalText(t []byte) error {
	cut := strings.TrimPrefix(string(t), invTypePrefix)
	cut = strings.TrimSuffix(cut, invTypeSuffix)
	if err := Spec(cut).Valid(); err != nil {
		return err
	}
	invT.Spec = Spec(cut)
	return nil
}

func (invT InventoryType) MarshalText() ([]byte, error) {
	return []byte(invT.String()), nil
}
