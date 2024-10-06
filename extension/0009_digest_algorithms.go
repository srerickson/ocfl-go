package extension

import (
	_ "embed"

	"github.com/srerickson/ocfl-go/digest"
)

const ext0009 = "0009-digest-algorithms"

var (
	Ext009AlgBlake2B_160 = &alg{id: "blake2b-160", ext: ExtraDigestAlgorithms{}}
	Ext009AlgBlake2B_256 = &alg{id: "blake2b-256", ext: ExtraDigestAlgorithms{}}
	Ext009AlgBlake2B_384 = &alg{id: "blake2b-384", ext: ExtraDigestAlgorithms{}}
	Ext009AlgSHA512_256  = &alg{id: "sha512/256", ext: ExtraDigestAlgorithms{}}
	Ext009AlgSize        = &alg{id: "size", ext: ExtraDigestAlgorithms{}}

	ext0009Algs = digest.NewRegistry(
		Ext009AlgBlake2B_160,
		Ext009AlgBlake2B_256,
		Ext009AlgBlake2B_384,
		Ext009AlgSHA512_256,
		Ext009AlgSize)

	//go:embed docs/0007-n-tuple-omit-prefix-storage-layout.md
	ext0009doc []byte
)

func Ext0009() Extension { return ExtraDigestAlgorithms{} }

// ExtraDigestAlgorithms implements 0009-digest-algorithms
type ExtraDigestAlgorithms struct{}

func (d ExtraDigestAlgorithms) Name() string { return ext0009 }

func (d ExtraDigestAlgorithms) Algorithms() digest.Registry {
	return ext0009Algs
}
