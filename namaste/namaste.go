package namaste

import (
	"fmt"
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
		return results, err
	}
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			fmt.Println(path)
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
		return results, err
	}
	return results, nil
}
