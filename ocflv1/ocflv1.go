// Package [ocflv1] provides an implementation of OCFL v1.0 and v1.1.
package ocflv1

import (
	"context"
	"fmt"

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

func (imp OCFL) Spec() ocfl.Spec {
	return imp.spec
}

func (imp OCFL) NewObject(ctx context.Context, root *ocfl.ObjectRoot, opts ...func(*ocfl.ObjectOptions)) (ocfl.Object, error) {
	obj := &Object{
		ObjectRoot: root,
	}
	for _, applyOpt := range opts {
		applyOpt(&obj.opts)
	}
	exists, err := obj.Exists(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		if err := obj.ObjectRoot.UnmarshalInventory(ctx, ".", &obj.Inventory); err != nil {
			return nil, err
		}
		invSpec := obj.Inventory.Type.Spec
		if invSpec != imp.spec {
			return nil, fmt.Errorf("object's OCFL specification (%q) is not supported: expected %q", string(invSpec), string(imp.spec))
		}
	}
	return obj, nil
}
