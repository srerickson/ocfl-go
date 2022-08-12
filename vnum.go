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

	V0 = VNum{} // warning: zero value is invalid
	V1 = VNum{num: 1}
)

// VNum represents an OCFL object version name ("v1","v02")
type VNum struct {
	num     int // positive integers 1,2,3..
	padding int // should be zero, but can be 1,2,3
}

// V returns a VNum for num with zero padding.
func V(num int) VNum {
	return VNum{num: num}
}

// ParseVNum parses strinv as a spec.Numer and sets the value pointed to be
// vn
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

// MustParseVNum is the same as ParseVnum except it panics instead of returning
// an error.
func MustParseVNum(str string) VNum {
	v := VNum{}
	err := ParseVNum(str, &v)
	if err != nil {
		panic(err)
	}
	return v
}

func (v VNum) Num() int {
	return v.num
}

func (v VNum) Padding() int {
	return v.padding
}

func (v VNum) Empty() bool {
	return v == V0
}

// Next returns the next spec.Number after v, with the same padding.
// An error is only returned if padding > 0 and next would overflow the padding
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
		return V0, errors.New("no previous version")
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

// VNumSeq returns a VNumSeq with v as last version
func (v VNum) VNumSeq() VNumSeq {
	if v.num == 0 {
		return VNumSeq{}
	}
	var nums VNumSeq = make([]VNum, v.num)
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

// VNumSeq is a slice of VNums
type VNumSeq []VNum

// VNum implements the sort.Interface interface
var _ sort.Interface = (*VNumSeq)(nil)

// Len implements sort.Interface on VNumSeq
func (vs VNumSeq) Len() int {
	return len(([]VNum)(vs))
}

// Less implements sort.Interface on VNumSeq
func (vs VNumSeq) Less(i, j int) bool {
	return (vs[i].num < vs[j].num)
}

// Swap implements sort.Interface on VNumSeq
func (vs VNumSeq) Swap(i, j int) {
	vs[i], vs[j] = vs[j], vs[i]
}

// Valid returns an error if vs if invalid
func (vs VNumSeq) Valid() error {
	if len(vs) == 0 {
		return ErrVerEmpty
	}
	if !sort.IsSorted(vs) {
		sort.Sort(vs)
	}
	padding := vs[0].padding
	for i := range vs {
		if vs[i].num != i+1 {
			return fmt.Errorf("version %d: %w", i+1, ErrVNumMissing)
		}
		if vs[i].padding != padding {
			return ErrVNumPadding
		}
	}
	// check that the last version doesn't have a padding overflow
	return vs.Head().Valid()
}

// Head returns the last VNum in vs
func (vs VNumSeq) Head() VNum {
	if len(vs) > 0 {
		return vs[len(vs)-1]
	}
	return V0
}

// Padding returns the padding for the VNums in vs
func (vs VNumSeq) Padding() int {
	if len(vs) > 0 {
		return vs[0].Padding()
	}
	return 0
}
