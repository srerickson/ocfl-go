package namaste

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

const (
	typePattern = `^0=%s$`
)

// SearchTypePattern returns a slice of directories under root with a namaste
// type matching pattern. Subdirectories of matching directories are not
// searched.
// Example: results, err := SearchTypePattern(`objects`, `ocfl_object.*`)
func SearchTypePattern(root string, pattern string) ([]string, error) {
	var results []string
	namasteRe, err := regexp.Compile(fmt.Sprintf(typePattern, pattern))
	if err != nil {
		return results, fmt.Errorf(`SearchTypePattern aborted: %s`, err)
	}
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			if len(namasteRe.FindStringSubmatch(info.Name())) > 0 {
				results = append(results, filepath.Dir(path))
				return filepath.SkipDir
			}
		}
		return nil
	}
	if err := filepath.Walk(root, walkFn); err != nil {
		return results, fmt.Errorf(`SearchTypePattern aborted: %s`, err)
	}
	return results, nil
}

// MatchTypePattern returns true if path is a directory with a namaste type tag
// matching pattern. An error is returned if path does not exist
func MatchTypePattern(path string, pattern string) (bool, error) {
	namasteRe, err := regexp.Compile(fmt.Sprintf(typePattern, pattern))
	if err != nil {
		return false, err
	}
	infos, err := ioutil.ReadDir(path)
	if err != nil {
		return false, err
	}
	for i := range infos {
		if len(namasteRe.FindString(infos[i].Name())) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// MatchTypePatternError returns an error if path does not match pattern
func MatchTypePatternError(path string, pattern string) error {
	match, err := MatchTypePattern(path, pattern)
	if err != nil {
		return err
	}
	if !match {
		return fmt.Errorf(`%s does not match %s`, path, pattern)
	}
	return nil
}

// SetType adds a namaste type tag to the directory at path. If path does not
// exist or is a directory, an error is returned.
func SetType(path string, dvalue string, fvalue string) error {
	fName := filepath.Join(path, fmt.Sprintf(`0=%s`, dvalue))
	return ioutil.WriteFile(fName, []byte(fvalue), 0644)
}
