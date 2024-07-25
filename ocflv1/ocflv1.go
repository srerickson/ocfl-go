// Package [ocflv1] provides an implementation of OCFL v1.0 and v1.1.
package ocflv1

import (
	"context"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/validation"
)

const (
	// defaults
	inventoryFile       = `inventory.json`
	contentDir          = `content`
	digestAlgorithm     = "sha512"
	extensionsDir       = "extensions"
	layoutName          = "ocfl_layout.json"
	storeRoot           = ocfl.NamasteTypeStore
	descriptionKey      = `description`
	extensionKey        = `extension`
	extensionConfigFile = "config.json"
)

var (
	ocflv1_0 = ocfl.Spec1_0

	// shorthand
	ec = validation.NewErrorCode
)

func init() {
	ocfl.RegisterOCLF(&OCFL{spec: ocfl.Spec1_0})
	ocfl.RegisterOCLF(&OCFL{spec: ocfl.Spec1_1})
}

// Implementation of OCFL v1.x
type OCFL struct {
	spec ocfl.Spec // 1.0 or 1.1
}

func (imp OCFL) Spec() ocfl.Spec { return imp.spec }

func (imp OCFL) OpenObject(ctx context.Context, fsys ocfl.FS, path string) (ocfl.SpecObject, error) {
	obj, err := OpenObject(ctx, fsys, path)
	if err != nil {
		return nil, err
	}
	// TODO: check obj spec?
	return obj, nil
}

// Commits creates or updates an object by adding a new object version based
// on the implementation.
func (imp OCFL) Commit(ctx context.Context, obj ocfl.SpecObject, c *ocfl.Commit) (ocfl.SpecObject, error) {
	//
	err := commit(ctx, obj, c, imp.spec)
	if err != nil {
		return nil, err
	}
	// FIXME: Commit should just return this
	return OpenObject(ctx, obj.FS(), obj.Path())
}
