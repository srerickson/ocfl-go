// Package [ocflv1] provides an implementation of OCFL v1.0 and v1.1.
package ocflv1

import (
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/validation"
)

const (
	// defaults
	inventoryFile       = `inventory.json`
	contentDir          = `content`
	digestAlgorithm     = "sha512"
	extensionsDir       = "extensions"
	layoutName          = "ocfl_layout.json"
	storeRoot           = ocfl.DeclStore
	descriptionKey      = `description`
	extensionKey        = `extension`
	extensionConfigFile = "config.json"
)

var (
	ocflv1_0    = ocfl.Spec{1, 0}
	ocflv1_1    = ocfl.Spec{1, 1}
	defaultSpec = ocflv1_1

	// supported versions
	ocflVerSupported = map[ocfl.Spec]bool{
		ocflv1_0: true,
		ocflv1_1: true,
	}

	// algs set to true can be used as digestAlgorithms
	// algorithms = map[ocfl.Alg]bool{
	// 	ocfl.SHA512:  true,
	// 	ocfl.SHA256:  true,
	// 	ocfl.SHA1:    false,
	// 	ocfl.MD5:     false,
	// 	ocfl.BLAKE2B: false,
	// }

	// shorthand
	ec = validation.NewErrorCode
)
