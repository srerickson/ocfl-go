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
	"io/fs"
	"regexp"
)

// func stringIn(a string, list []string) bool {
// 	for i := range list {
// 		if a == list[i] {
// 			return true
// 		}
// 	}
// 	return false
// }

// minusString returns slice of strings in a that aren't in b
func minusStrings(a []string, b []string) []string {
	var minus []string //in a but not in b
	for i := range a {
		var found bool
		for j := range b {
			if a[i] == b[j] {
				found = true
			}
		}
		if !found {
			minus = append(minus, a[i])
		}
	}
	return minus
}

type dirEntry []fs.DirEntry

type dirMatch struct {
	ReqFiles   []string
	OptFiles   []string
	ReqDirs    []string
	OptDirs    []string
	FileRegexp *regexp.Regexp
	DirRegexp  *regexp.Regexp
}

var (
	errDirMatchMissingFile = errors.New("file is missing")
	errDirMatchInvalidFile = errors.New("file in invalid")
	errDirMatchMissingDir  = errors.New("directory is missing")
	errDirMatchInvalidDir  = errors.New("directory is invalid")
)

func (match dirMatch) Match(items []fs.DirEntry) error {
	var dirs []string
	var files []string
	for _, d := range items {
		name := d.Name()
		if d.Type().IsDir() {
			dirs = append(dirs, name)
		}
		if d.Type().IsRegular() {
			files = append(files, name)
		}
	}
	// directories
	missing := minusStrings(match.ReqDirs, dirs)
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", errDirMatchMissingDir, missing[0])
	}
	extra := minusStrings(dirs, match.ReqDirs)
	extra = minusStrings(extra, match.OptDirs)
	if match.DirRegexp != nil {
		for _, e := range extra {
			if !match.DirRegexp.MatchString(e) {
				return fmt.Errorf("%w: %s", errDirMatchInvalidDir, e)
			}
		}
	} else if len(extra) > 0 {
		return fmt.Errorf("%w: %s", errDirMatchInvalidDir, extra[0])
	}
	// files
	missing = minusStrings(match.ReqFiles, files)
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", errDirMatchMissingFile, missing[0])
	}
	extra = minusStrings(files, match.ReqFiles)
	extra = minusStrings(extra, match.OptFiles)
	if match.FileRegexp != nil {
		for _, e := range extra {
			if !match.FileRegexp.MatchString(e) {
				return fmt.Errorf("%w: %s", errDirMatchInvalidFile, e)
			}
		}
	} else if len(extra) > 0 {
		return fmt.Errorf("%w: %s", errDirMatchInvalidFile, extra[0])
	}
	return nil
}
