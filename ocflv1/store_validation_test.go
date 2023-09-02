package ocflv1_test

import (
	"archive/zip"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
)

func TestStoreValidation(t *testing.T) {
	specs := []string{`1.0`}
	for _, spec := range specs {
		t.Run(spec, func(t *testing.T) {

			fixturePath := filepath.Join(`..`, `testdata`, `store-fixtures`, spec)
			goodPath := filepath.Join(fixturePath, `good-stores`)
			badPath := filepath.Join(fixturePath, `bad-stores`)
			warnPath := filepath.Join(fixturePath, `warn-stores`)

			t.Run("Valid storage roots", func(t *testing.T) {
				dirs, err := os.ReadDir(goodPath)
				if err != nil {
					t.Fatal(err)
				}
				for _, dir := range dirs {
					name := dir.Name()
					if dir.Type().IsRegular() && !strings.HasSuffix(name, ".zip") {
						continue
					}
					t.Run(name, func(t *testing.T) {
						var fsys ocfl.FS
						if dir.Type().IsRegular() {
							zFS, err := zip.OpenReader(filepath.Join(goodPath, name))
							if err != nil {
								t.Fatal(err)
							}
							defer zFS.Close()
							fsys = ocfl.NewFS(zFS)
						} else {
							fsys = ocfl.DirFS(filepath.Join(goodPath, name))
						}
						result := ocflv1.ValidateStore(context.Background(), fsys, `.`)
						if result.Err() != nil {
							t.Error(err)
						}
						if len(result.Warn()) != 0 {
							for _, w := range result.Warn() {
								t.Fatalf("unexpected warning: %s", w.Error())
							}
						}

					})
				}
			})

			t.Run("Invalid storage roots", func(t *testing.T) {
				fsys := os.DirFS(badPath)
				dirs, err := fs.ReadDir(fsys, ".")
				if err != nil {
					t.Fatal(err)
				}
				for _, dir := range dirs {
					name := dir.Name()
					if dir.Type().IsRegular() && !strings.HasSuffix(name, ".zip") {
						continue
					}
					t.Run(dir.Name(), func(t *testing.T) {
						var fsys ocfl.FS
						if dir.Type().IsRegular() {
							zFS, err := zip.OpenReader(filepath.Join(badPath, name))
							if err != nil {
								t.Fatal(err)
							}
							defer zFS.Close()
							fsys = ocfl.NewFS(zFS)
						} else {
							fsys = ocfl.DirFS(filepath.Join(badPath, name))
						}
						result := ocflv1.ValidateStore(context.Background(), fsys, `.`)
						if result.Err() == nil {
							t.Error(`validated but shouldn't`)
						}

					})
				}
			})

			t.Run("Warning storage roots", func(t *testing.T) {
				fsys := os.DirFS(warnPath)
				dirs, err := fs.ReadDir(fsys, ".")
				if err != nil {
					t.Fatal(err)
				}
				for _, dir := range dirs {
					name := dir.Name()
					if dir.Type().IsRegular() && !strings.HasSuffix(name, ".zip") {
						continue
					}
					t.Run(dir.Name(), func(t *testing.T) {
						var fsys ocfl.FS
						if dir.Type().IsRegular() {
							zFS, err := zip.OpenReader(filepath.Join(warnPath, name))
							if err != nil {
								t.Fatal(err)
							}
							defer zFS.Close()
							fsys = ocfl.NewFS(zFS)
						} else {
							fsys = ocfl.DirFS(filepath.Join(warnPath, name))
						}
						result := ocflv1.ValidateStore(context.Background(), fsys, `.`)
						if err := result.Err(); err != nil {
							t.Error(err)
						}
						if len(result.Warn()) == 0 {
							t.Fatal("expected warnings, got none")
						}
					})
				}
			})

		})
	}
}
