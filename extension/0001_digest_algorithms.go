package extension

import (
	_ "embed"

	"github.com/srerickson/ocfl-go/digest"
)

const ext0001name = "0009-digest-algorithms"

//go:embed docs/0001-digest-algorithms.md
var ext0001doc []byte

func Ext0001() AlgorithmRegistry {
	ext := algRegistry{
		Base: Base{ExtensionName: ext0009name},
	}
	ext.algs = digest.NewRegistry(
		&alg{id: "blake2b-160", ext: ext},
		&alg{id: "blake2b-256", ext: ext},
		&alg{id: "blake2b-384", ext: ext},
		&alg{id: "sha512/256", ext: ext},
	)
	return ext
}
