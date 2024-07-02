// Package [ocflv1] provides an implementation of OCFL v1.0 and v1.1.
package ocflv1

import (
	"context"
	"errors"

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
	ocflv1_0    = ocfl.Spec1_0
	ocflv1_1    = ocfl.Spec1_1
	defaultSpec = ocflv1_1

	// supported versions
	ocflVerSupported = map[ocfl.Spec]bool{
		ocflv1_0: true,
		ocflv1_1: true,
	}

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

func (imp OCFL) Commit(ctx context.Context, obj ocfl.SpecObject, commit *ocfl.Commit) (ocfl.SpecObject, error) {
	writeFS, ok := obj.FS().(ocfl.WriteFS)
	if !ok {
		return nil, errors.New("object's backing file system doesn't support write operations")
	}
	err := Commit(ctx, writeFS, obj.Path(), commit.ID, commit.Stage,
		WithUser(&commit.User),
		WithMessage(commit.Message),
		WithOCFLSpec(imp.spec),
		WithAllowUnchanged(commit.AllowUnchanged),
		WithHEAD(commit.NewHEAD),
	)
	if err != nil {
		return nil, err
	}
	// FIXME: Commit should just return this
	return OpenObject(ctx, obj.FS(), obj.Path())
}
