package ocflv1

import (
	"testing"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
)

func TestLedger(t *testing.T) {
	var ledg pathLedger
	is := is.New(t)
	err := ledg.addPathDigest("tmp.txt", digest.SHA512id, "abdc", ocfl.V(1), inFixity|inRootInv)
	is.NoErr(err)
	info, exists := ledg.paths["tmp.txt"]
	is.True(exists)
	is.True(info.referencedIn(ocfl.V(1), inRootInv))
	is.True(info.referencedIn(ocfl.V(1), inFixity))
	is.True(info.referencedIn(ocfl.V(1), inRootInv|inFixity))
	is.True(!info.referencedIn(ocfl.V(1), 0))
	is.True(!info.referencedIn(ocfl.V(1), inVerInv))
	is.True(!info.referencedIn(ocfl.V(1), inManifest))
	is.True(!info.referencedIn(ocfl.V(1), inManifest|inVerInv))
	dInfo, exists := info.digests[digest.SHA512id]
	is.True(exists)
	dInfo.digest = "abdc"
	is.True(dInfo.locs[ocfl.V(1)].InFixity())
	is.True(!dInfo.locs[ocfl.V(1)].InVerInv())
	is.True(dInfo.locs[ocfl.V(1)].InRoot())
	is.True(!dInfo.locs[ocfl.V(1)].InManifest())
}
