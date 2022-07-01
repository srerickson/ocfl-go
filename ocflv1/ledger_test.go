package ocflv1

import (
	"testing"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/object"
)

func TestLedger(t *testing.T) {
	var ledg pathLedger
	is := is.New(t)
	err := ledg.addPathDigest("tmp.txt", digest.SHA512, "abdc", object.V1, inFixity|inRootInv)
	is.NoErr(err)
	info, exists := ledg.paths["tmp.txt"]
	is.True(exists)
	is.True(info.referencedIn(object.V1, inRootInv))
	is.True(info.referencedIn(object.V1, inFixity))
	is.True(info.referencedIn(object.V1, inRootInv|inFixity))
	is.True(!info.referencedIn(object.V1, 0))
	is.True(!info.referencedIn(object.V1, inVerInv))
	is.True(!info.referencedIn(object.V1, inManifest))
	is.True(!info.referencedIn(object.V1, inManifest|inVerInv))
	dInfo, exists := info.digests[digest.SHA512]
	is.True(exists)
	dInfo.digest = "abdc"
	is.True(dInfo.locs[object.V1].InFixity())
	is.True(!dInfo.locs[object.V1].InVerInv())
	is.True(dInfo.locs[object.V1].InRoot())
	is.True(!dInfo.locs[object.V1].InManifest())
}
