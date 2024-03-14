// Package [ocflv1] provides an implementation of OCFL v1.0 and v1.1.
package ocflv1

import (
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
