package extension

import (
	"github.com/srerickson/ocfl-go/digest"
)

const ext0001 = "0001-digest-algorithms"

func Ext0001() Extension {
	ext := &algRegistry{
		Base: Base{ExtensionName: ext0001},
	}
	ext.algs = digest.NewRegistry(
		&alg{id: "blake2b-160", ext: ext},
		&alg{id: "blake2b-256", ext: ext},
		&alg{id: "blake2b-384", ext: ext},
		&alg{id: "sha512/256", ext: ext},
	)
	return ext
}
