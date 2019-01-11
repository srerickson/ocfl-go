package ocfl

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

var versionFormats = map[string]*regexp.Regexp{
	`padded`:   regexp.MustCompile(`v0\d+`),
	`unpadded`: regexp.MustCompile(`v[1-9]\d*`),
}

const version1 = `v1`

// returns version format name (padded or unpadded)
func versionFormat(name string) string {
	for style, re := range versionFormats {
		if re.MatchString(name) {
			return style
		}
	}
	return ``
}

// returns the amount of padding in the version name
func versionPadding(name string) int {
	if versionFormat(name) == `padded` {
		return len(name) - 1
	}
	return 0
}

// returns integer representation of the version (0 is a failure)
func versionInt(name string) int {
	if name == `` {
		return 0
	}
	i, _ := strconv.Atoi(name[1:])
	return i
}

func versionGen(num int, padding int) string {
	format := fmt.Sprintf("v%%0%dd", padding)
	return fmt.Sprintf(format, num)
}

// returns next version name in the style of the given version name
func nextVersionLike(prev string) (string, error) {
	padding := versionPadding(prev)
	next := versionInt(prev) + 1
	if next == 1 {
		return ``, errors.New(`invalid version format`)
	}
	if padding > 0 && float64(next) >= math.Pow10(padding-1) {
		return ``, errors.New(`version format doesn't allow additional versions`)
	}
	return versionGen(next, padding), nil
}
