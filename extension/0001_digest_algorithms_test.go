package extension_test

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/extension"
)

func TestExt0001DigestAlgorithms(t *testing.T) {
	t.Run("digest values", func(t *testing.T) {
		algExt, ok := extension.Ext0001().(extension.AlgorithmRegistry)
		be.True(t, ok)
		hashData := []byte("hello world")
		table := map[string]string{
			"blake2b-160": "70e8ece5e293e1bda064deef6b080edde357010f",
			"blake2b-256": "256c83b297114d201b30179f3f0ef0cace9783622da5974326b436178aeef610",
			"blake2b-384": "8c653f8c9c9aa2177fb6f8cf5bb914828faa032d7b486c8150663d3f6524b086784f8e62693171ac51fc80b7d2cbb12b",
			"sha512/256":  "0ac561fac838104e3f2e4ad107b4bee3e938bf15f2b15f009ccccd61a913f017",
		}
		for id, digest := range table {
			t.Run(id, func(t *testing.T) {
				digester := algExt.Algorithms().MustGet(id).Digester()
				digester.Write(hashData)
				be.Equal(t, digest, digester.String())
			})
		}
	})
}
