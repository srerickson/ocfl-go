package ocfl

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	invTypePrefix = "https://ocfl.io/"
	invTypeSuffix = "/spec/#inventory"
)

var (
	ErrSpecInvalid = errors.New("invalid OCFL spec version")
)

// Spec represent an OCFL specification number
type Spec [2]int

func (num *Spec) UnmarshalText(text []byte) error {
	return ParseSpec(string(text), num)
}

func (num Spec) MarshalText() ([]byte, error) {
	return []byte(num.String()), nil
}

func ParseSpec(v string, n *Spec) error {
	if len(v) < 3 {
		return fmt.Errorf("%w: %s", ErrSpecInvalid, v)
	}
	a, b, found := strings.Cut(v, `.`)
	if !found {
		return fmt.Errorf("%w: %s", ErrSpecInvalid, v)
	}
	if len(a) < 1 || a[0] == '0' || len(b) < 1 || (len(b) > 1 && b[0] == '0') {
		return fmt.Errorf("%w: %s", ErrSpecInvalid, v)
	}
	maj, err := strconv.Atoi(a)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSpecInvalid, v)
	}
	min, err := strconv.Atoi(b)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSpecInvalid, v)
	}
	n[0] = maj
	n[1] = min
	return nil
}

func MustParseSpec(v string) Spec {
	var n Spec
	err := ParseSpec(v, &n)
	if err != nil {
		panic(err)
	}
	return n
}

func (n Spec) String() string {
	return fmt.Sprintf("%d.%d", n[0], n[1])
}

// Cmp compares Spec v1 to another v2.
// - If v1 is less than v2, returns -1.
// - If v1 is the same as v2, returns 0
// - If v1 is greater than v2, returns 1
func (v1 Spec) Cmp(v2 Spec) int {
	var diff int
	if v1[0] == v2[0] {
		diff = v1[1] - v2[1]
	} else {
		diff = v1[0] - v2[0]
	}
	if diff > 0 {
		return 1
	} else if diff < 0 {
		return -1
	}
	return 0
}

func (n Spec) Empty() bool {
	return n == Spec{}
}

// AsInvType returns n as an InventoryType
func (n Spec) AsInvType() InvType {
	return InvType{Spec: n}

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
	return ParseSpec(cut, &invT.Spec)
}

func (invT InvType) MarshalText() ([]byte, error) {
	return []byte(invT.String()), nil
}