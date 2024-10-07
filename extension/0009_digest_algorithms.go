package extension

import (
	"github.com/srerickson/ocfl-go/digest"
)

const ext0009 = "0009-digest-algorithms"

func Ext0009() Extension {
	ext := algRegistry{
		Base: Base{ExtensionName: ext0009},
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
