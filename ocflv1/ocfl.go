package ocflv1

import (
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/namaste"
	"github.com/srerickson/ocfl/spec"
	"github.com/srerickson/ocfl/validation"
)

const (
	// defaults
	inventoryFile   = `inventory.json`
	contentDir      = `content`
	digestAlgorithm = "sha512"
	extensionsDir   = "extensions"
	layoutName      = "ocfl_layout.json"
	storeRoot       = namaste.StoreType
)

var (
	ocflv1_0 = spec.Num{1, 0}
	ocflv1_1 = spec.Num{1, 1}

	// supported versions
	ocflVerSupported = map[spec.Num]bool{
		ocflv1_0: true,
		ocflv1_1: false,
	}

	// algs set to true can be used as digestAlgorithms
	algorithms = map[digest.Alg]bool{
		digest.SHA512:  true,
		digest.SHA256:  true,
		digest.SHA224:  false,
		digest.SHA1:    false,
		digest.MD5:     false,
		digest.BLAKE2B: false,
	}

	// shorthand
	ec = validation.NewErrorCode
)
