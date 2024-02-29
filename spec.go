package ocfl

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

const (
	Spec1_0 = Spec("1.0")
	Spec1_1 = Spec("1.1")

	invTypePrefix = "https://ocfl.io/"
	invTypeSuffix = "/spec/#inventory"
	specsDir      = "specs"
)

var (
	specs = []Spec{Spec1_0, Spec1_1}

	ErrSpecInvalid  = errors.New("invalid OCFL spec version")
	ErrSpecNotFound = errors.New("OCFL spec file not found")
)

//go:embed specs/*
var specFS embed.FS

// Spec represent an OCFL specification number
type Spec string

func (s *Spec) UnmarshalText(text []byte) error {
	newS, err := ParseSpec(string(text))
	if err != nil {
		return err
	}
	*s = newS
	return nil
}

func (num Spec) MarshalText() ([]byte, error) {
	return []byte(num.String()), nil
}

func ParseSpec(v string) (Spec, error) {
	s := Spec(v)
	if slices.Index(specs, s) < 0 {
		return Spec(""), ErrSpecInvalid
	}
	return s, nil
}

func MustParseSpec(v string) Spec {
	s, err := ParseSpec(v)
	if err != nil {
		panic(err)
	}
	return s
}

func (s Spec) String() string { return string(s) }

// Cmp compares Spec v1 to another v2.
// - If v1 is less than v2, returns -1.
// - If v1 is the same as v2, returns 0
// - If v1 is greater than v2, returns 1
func (v1 Spec) Cmp(v2 Spec) int {
	return slices.Index(specs, v1) - slices.Index(specs, v2)
}

func (n Spec) Empty() bool {
	return n == Spec("")
}

// AsInvType returns n as an InventoryType
func (n Spec) AsInvType() InvType {
	return InvType{Spec: n}
}

func WriteSpecFile(ctx context.Context, fsys WriteFS, dir string, n Spec) (string, error) {
	glob := specsDir + "/" + "ocfl_" + n.String() + ".*"
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

// InvType represents an inventory type string
// for example: https://ocfl.io/1.0/spec/#inventory
type InvType struct {
	Spec
}

func (inv InvType) String() string {
	return invTypePrefix + inv.Spec.String() + invTypeSuffix
}

func (invT *InvType) UnmarshalText(t []byte) error {
	cut := strings.TrimPrefix(string(t), invTypePrefix)
	cut = strings.TrimSuffix(cut, invTypeSuffix)
	s, err := ParseSpec(cut)
	if err != nil {
		return err
	}
	invT.Spec = s
	return nil
}

func (invT InvType) MarshalText() ([]byte, error) {
	return []byte(invT.String()), nil
}
