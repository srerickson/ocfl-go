package ocfl

import (
	"encoding"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
)

var (
	ErrVNumInvalid = errors.New(`invalid version`)
	ErrVNumPadding = errors.New(`inconsistent version padding in version sequence`)
	ErrVNumMissing = errors.New(`missing version in version sequence`)
	ErrVerEmpty    = errors.New("no versions found")

	// Some functions in this package use the zero value VNum to indicate the
	// most recent, "head" version.
	Head = VNum{}
)

// VNum represents an OCFL object version number (e.g., "v1", "v02"). A VNum has
// a sequence number (1,2,3...) and a padding number, which defaults to zero.
// The padding is the maximum number of numeric digits the version number can
// include (a padding of 0 is no maximum). The padding value constrains the
// maximum valid sequence number.
type VNum struct {
	num     int // positive integers 1,2,3..
	padding int // should be zero, but can be 2,3,4
}

// V returns a new Vnum. The first argument is a sequence number. An optional
// second argument can be used to set the padding. Additional arguments are
// ignored. Without any arguments, V() returns a zero value VNum.
func V(ns ...int) VNum {
	switch len(ns) {
	case 0:
		return VNum{}
	case 1:
		return VNum{num: ns[0]}
	default:
		return VNum{num: ns[0], padding: ns[1]}
	}
}

// ParseVNum parses string as an a VNum and sets the value referenced by vn.
func ParseVNum(v string, vn *VNum) error {
	var n, p int
	var nonzero bool
	var err error
	if len(v) < 2 {
		return fmt.Errorf("%s: %w", v, ErrVNumInvalid)
	}
	if v[0] != 'v' {
		return fmt.Errorf("%s: %w", v, ErrVNumInvalid)
	}
	if v[1] == '0' {
		p = len(v) - 1
	}
	for i := 1; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return fmt.Errorf("%s: %w", v, ErrVNumInvalid)
		}
		if v[i] != '0' {
			nonzero = true
		}
	}
	if !nonzero {
		return fmt.Errorf("%s: %w", v, ErrVNumInvalid)
	}
	if n, err = strconv.Atoi(v[1:]); err != nil {
		return fmt.Errorf("%s: %w", v, ErrVNumInvalid)
	}
	vn.num = n
	vn.padding = p
	return nil
}

// MustParseVNum parses str as a VNUm and returns a new VNum. It panics if str
// cannot be parsed as a VNum.
func MustParseVNum(str string) VNum {
	v := VNum{}
	err := ParseVNum(str, &v)
	if err != nil {
		panic(err)
	}
	return v
}

// Num returns v's number as an int
func (v VNum) Num() int {
	return v.num
}

// Padding returns v's padding number.
func (v VNum) Padding() int {
	return v.padding
}

// IsZero returns if v is the zero value
func (v VNum) IsZero() bool {
	return v == Head
}

// First returns true if v is a version 1.
func (v VNum) First() bool {
	return v.num == 1
}

// Next returns the next ocfl.VNum after v with the same padding. A non-nil
// error is returned if padding > 0 and next would overflow the padding
func (v VNum) Next() (VNum, error) {
	next := VNum{
		num:     v.num + 1,
		padding: v.padding,
	}
	if next.paddingOverflow() {
		err := fmt.Errorf("next version: padding overflow: %w", ErrVNumInvalid)
		return VNum{}, err
	}
	return next, nil
}

// Prev returns the previous version before v, with the same padding.
// An error is returned if v.Num() == 1
func (v VNum) Prev() (VNum, error) {
	if v.num == 1 {
		return Head, errors.New("no previous version")
	}
	return VNum{
		num:     v.num - 1,
		padding: v.padding,
	}, nil
}

// String returns string representation of v
func (v VNum) String() string {
	format := fmt.Sprintf("v%%0%dd", v.padding)
	return fmt.Sprintf(format, v.num)
}

// Valid returns an error if v is invalid
func (v VNum) Valid() error {
	if v.num <= 0 || v.paddingOverflow() {
		return fmt.Errorf("%w: num=%d, padding=%d",
			ErrVNumInvalid, v.num, v.padding)
	}
	return nil
}

// paddingOverflow indicates v.padding is too small for v.num
func (v VNum) paddingOverflow() bool {
	return v.padding > 0 && v.num >= int(math.Pow10(v.padding-1))
}

// Lineage returns a VNums with v as the head.
func (v VNum) Lineage() VNums {
	if v.num == 0 {
		return VNums{}
	}
	var nums VNums = make([]VNum, v.num)
	for i := 0; i < v.num; i++ {
		nums[i] = VNum{i + 1, v.padding}
	}
	return nums
}

// Interfaces VNum implements
var _ encoding.TextUnmarshaler = (*VNum)(nil)
var _ encoding.TextMarshaler = (*VNum)(nil)

func (v *VNum) UnmarshalText(text []byte) error {
	err := ParseVNum(string(text), v)
	if err != nil {
		return err
	}
	return nil
}

func (v VNum) MarshalText() ([]byte, error) {
	if err := v.Valid(); err != nil {
		return nil, err
	}
	return []byte(v.String()), nil
}

// VNums is a slice of VNum elements
type VNums []VNum

// Valid returns a non-nill error if VNums is empty, is not a continuous
// sequence (1,2,3...), has inconsistent padding or padding overflow.
func (vs VNums) Valid() error {
	if len(vs) == 0 {
		return ErrVerEmpty
	}
	if !sort.IsSorted(vs) {
		sort.Sort(vs)
	}
	padding := vs[0].padding
	for i := range vs {
		if vs[i].num != i+1 {
			return fmt.Errorf("%w: %s", ErrVNumMissing, V(i+1, padding))
		}
		if vs[i].padding != padding {
			return ErrVNumPadding
		}
	}
	// check that the last version doesn't have a padding overflow
	return vs.Head().Valid()
}

// Head returns the last VNum in vs.
func (vs VNums) Head() VNum {
	if len(vs) > 0 {
		return vs[len(vs)-1]
	}
	return VNum{}
}

// Padding returns the padding for the VNums in vs
func (vs VNums) Padding() int {
	if len(vs) > 0 {
		return vs[0].Padding()
	}
	return 0
}

// VNums implements the sort.Interface interface
var _ sort.Interface = (*VNums)(nil)

// Len implements sort.Interface on VNums
func (vs VNums) Len() int {
	return len(([]VNum)(vs))
}

// Less implements sort.Interface on VNums
func (vs VNums) Less(i, j int) bool {
	return (vs[i].num < vs[j].num)
}

// Swap implements sort.Interface on VNums
func (vs VNums) Swap(i, j int) {
	vs[i], vs[j] = vs[j], vs[i]
}
