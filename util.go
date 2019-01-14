// Copyright 2019 Seth R. Erickson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocfl

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

const version1 = `v1`

var versionFormats = map[string]*regexp.Regexp{
	`padded`:   regexp.MustCompile(`v0\d+`),
	`unpadded`: regexp.MustCompile(`v[1-9]\d*`),
}

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

// generates version with num and padding. return error if num is too big
// for padding
func versionGen(num int, padding int) (string, error) {
	if padding > 0 && float64(num) >= math.Pow10(padding-1) {
		return ``, errors.New(`version padding overflow`)
	}
	format := fmt.Sprintf("v%%0%dd", padding)
	return fmt.Sprintf(format, num), nil
}

// returns next version name in the style of the given version name
func nextVersionLike(prev string) (string, error) {
	padding := versionPadding(prev)
	next := versionInt(prev) + 1
	if next == 1 {
		return ``, errors.New(`invalid version format`)
	}
	return versionGen(next, padding)
}

func stringIn(a string, list []string) bool {
	for i := range list {
		if a == list[i] {
			return true
		}
	}
	return false
}
