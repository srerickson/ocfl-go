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
		&alg{Algorithm: digest.BLAKE2B_160, ext: ext},
		&alg{Algorithm: digest.BLAKE2B_256, ext: ext},
		&alg{Algorithm: digest.BLAKE2B_384, ext: ext},
		&alg{Algorithm: digest.SHA512_256, ext: ext},
	)
	return ext
}
