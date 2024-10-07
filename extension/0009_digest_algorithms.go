package extension

import (
	_ "embed"

	"github.com/srerickson/ocfl-go/digest"
)

const ext0009name = "0009-digest-algorithms"

//go:embed docs/0009-digest-algorithms.md
var ext0009doc []byte

func Ext0009() AlgorithmRegistry {
	ext := algRegistry{
		Base: Base{ExtensionName: ext0009name},
	}
	ext.algs = digest.NewRegistry(
		&alg{id: "blake2b-160", ext: ext},
		&alg{id: "blake2b-256", ext: ext},
		&alg{id: "blake2b-384", ext: ext},
		&alg{id: "sha512/256", ext: ext},
		&alg{id: "size", ext: ext},
	)
	return ext
}
