package namaste

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

// SearchTypePattern returns a slice of directories under root with a namaste
// type matching pattern. Subdirectories of matching directories are not
// searched.
// Example: results, err := SearchTypePattern(`objects`, `ocfl_object.*`)
func SearchTypePattern(root string, pattern string) ([]string, error) {
	var results []string
	namasteRe, err := regexp.Compile(fmt.Sprintf(`^0=%s$`, pattern))
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

// SetType adds a namaste type tag to the directory at path. If path does not
// exist or is a directory, an error is returned.
func SetType(path string, tvalue string, fvalue string) error {
	fName := filepath.Join(path, fmt.Sprintf(`0=%s`, tvalue))
	return ioutil.WriteFile(fName, []byte(fvalue), 0644)
}
