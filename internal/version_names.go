package internal

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
)

//const version1 = `v1`

type versionFmt int

const (
	vUnknownFmt  versionFmt = -1
	vUnpaddedFmt versionFmt = 0
	vPaddedFmt   versionFmt = 1
)

var ErrVersionInvalid = errors.New(`invalid version name format`)

var vFmtRegexps = map[versionFmt]*regexp.Regexp{
	vPaddedFmt:   regexp.MustCompile(`^v0\d+$`),
	vUnpaddedFmt: regexp.MustCompile(`^v[1-9]\d*$`),
}

// returns version format
func versionFormat(name string) versionFmt {
	for style, re := range vFmtRegexps {
		if re.MatchString(name) {
			return style
		}
	}
	return vUnknownFmt
}

// returns the amount of padding in the version name
func versionPadding(name string) (int, error) {
	if versionFormat(name) == vUnpaddedFmt {
		return 0, nil
	}
	if versionFormat(name) == vPaddedFmt {
		return len(name) - 1, nil
	}
	return -1, fmt.Errorf("%w: %s", ErrVersionInvalid, name)
}

// returns integer representation of the version
func versionInt(name string) (int, error) {
	if name == "" {
		return 0, fmt.Errorf("%w: %s", ErrVersionInvalid, name)
	}
	return strconv.Atoi(name[1:])
}

// versionParse returns number and padding of the version string
func versionParse(name string) (int, int, error) {
	pad, err := versionPadding(name)
	if err != nil {
		return 0, 0, err
	}
	num, err := versionInt(name)
	if err != nil {
		return 0, 0, err
	}
	if num == 0 {
		return 0, 0, fmt.Errorf("%w: %s", ErrVersionInvalid, name)
	}
	return num, pad, nil
}

// generates version with num and padding. return error if num is too big
// for padding
func versionGen(num int, padding int) (string, error) {
	if num <= 0 {
		return ``, errors.New(`version must be >0`)
	}
	if padding < 0 {
		return ``, errors.New(`padding must be >= 0`)
	}
	if padding > 0 && num >= int(math.Pow10(padding-1)) {
		return ``, errors.New(`version padding overflow`)
	}
	format := fmt.Sprintf("v%%0%dd", padding)
	return fmt.Sprintf(format, num), nil
}

// returns next version name in the style of the given version name
func nextVersionLike(prev string) (string, error) {
	padding, err := versionPadding(prev)
	if err != nil {
		return ``, err
	}
	v, err := versionInt(prev)
	if err != nil {
		return ``, err
	}
	if v == 0 {
		return ``, fmt.Errorf("%w: %s", ErrVersionInvalid, prev)
	}
	v++
	return versionGen(v, padding)
}

// is the sequence of versions names ok?
func versionSeqValid(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf(`no versions: %w`, &ErrE008)
	}
	var padding int
	var nums = make([]int, 0, len(names))
	for i, name := range names {
		v, p, err := versionParse(name)
		if err != nil {
			return err
		}
		if i == 0 {
			padding = p
		} else if padding != p {
			return &ValidationErr{
				err:  fmt.Errorf(`inconsistent version padding: %s`, name),
				code: &ErrE012,
			}
		}
		nums = append(nums, v)
	}
	sort.IntSlice(nums).Sort()
	for i, v := range nums {
		expectedV := i + 1
		if v != expectedV {
			if i == 0 {
				return &ValidationErr{
					err:  fmt.Errorf(`missing version 1`),
					code: &ErrE009,
				}
			}
			return &ValidationErr{
				err:  fmt.Errorf(`missing version %d`, expectedV),
				code: &ErrE010,
			}
		}
	}
	return nil
}
