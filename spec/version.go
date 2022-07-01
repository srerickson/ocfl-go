package spec

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrInvalid = errors.New("invalid OCFL spec version")
	Empty      = Num{}
)

// Num represent an OCFL Specification spec.Numer
type Num [2]int

func (num *Num) UnmarshalText(text []byte) error {
	return Parse(string(text), num)
}

func (num Num) MarshalText() ([]byte, error) {
	return []byte(num.String()), nil
}

func Parse(v string, n *Num) error {
	if len(v) < 3 {
		return fmt.Errorf("%w: %s", ErrInvalid, v)
	}
	a, b, found := strings.Cut(v, `.`)
	if !found {
		return fmt.Errorf("%w: %s", ErrInvalid, v)
	}
	if len(a) < 1 || a[0] == '0' || len(b) < 1 || (len(b) > 1 && b[0] == '0') {
		return fmt.Errorf("%w: %s", ErrInvalid, v)
	}
	maj, err := strconv.Atoi(a)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, v)
	}
	min, err := strconv.Atoi(b)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, v)
	}
	n[0] = maj
	n[1] = min
	return nil
}

func MustParse(v string) Num {
	var n Num
	err := Parse(v, &n)
	if err != nil {
		panic(err)
	}
	return n
}

func (n Num) String() string {
	return fmt.Sprintf("%d.%d", n[0], n[1])
}

// Cmp compares Num v1 to another v2.
// - If v1 is less than v2, returns -1.
// - If v1 is the same as v2, returns 0
// - If v1 is greater than v2, returns 1
func (v1 Num) Cmp(v2 Num) int {
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

func (n Num) Empty() bool {
	return n == Empty
}

// AsInventoryType returns n as an InventoryType
func (n Num) AsInventoryType() InventoryType {
	return InventoryType{Num: n}

}
