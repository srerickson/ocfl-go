package extension

import (
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	ext0003         = "0003-hash-and-id-n-tuple-storage-layout"
	numberOfTuples  = "numberOfTuples"
	digestAlgorithm = "digestAlgorithm"
	tupleSize       = "tupleSize"
	lowerhex        = "0123456789abcdef"
)

//go:embed docs/0003-hash-and-id-n-tuple-storage-layout.md
var ext0003doc []byte

// LayoutHashIDTuple implements 0003-hash-and-id-n-tuple-storage-layout
type LayoutHashIDTuple struct {
	Base
	DigestAlgorithm string `json:"digestAlgorithm"`
	TupleSize       int    `json:"tupleSize"`
	TupleNum        int    `json:"numberOfTuples"`
}

var _ (Layout) = (*LayoutHashIDTuple)(nil)
var _ (Extension) = (*LayoutHashIDTuple)(nil)

// Ext0003 returns a new instance of 0003-hash-and-id-n-tuple-storage-layout with default values
func Ext0003() Extension {
	return &LayoutHashIDTuple{
		Base:            Base{ExtensionName: ext0003},
		DigestAlgorithm: "sha256",
		TupleSize:       3,
		TupleNum:        3,
	}
}

func (l LayoutHashIDTuple) Documentation() []byte { return ext0003doc }

func (l LayoutHashIDTuple) Valid() error {
	h := getHash(l.DigestAlgorithm)
	if h == nil {
		return fmt.Errorf("unknown digest algorithm: %q", l.DigestAlgorithm)
	}
	if l.TupleSize == 0 && l.TupleNum != 0 {
		return errors.New(numberOfTuples + " must be 0 if " + tupleSize + " is 0")
	}
	if l.TupleNum == 0 && l.TupleSize != 0 {
		return errors.New(tupleSize + " must be 0 if " + numberOfTuples + " is 0")
	}
	// h.Size()*2 is number of characters in hex encoded digest
	if l.TupleNum*(l.TupleSize) > h.Size()*2 {
		err := errors.New("product of " + tupleSize + " and " + numberOfTuples + " is more then hash length for " + l.DigestAlgorithm)
		return err
	}
	return nil
}

func (l LayoutHashIDTuple) Resolve(id string) (string, error) {
	if err := l.Valid(); err != nil {
		return "", err
	}
	h := getHash(l.DigestAlgorithm)
	tupSize, tupNum := l.TupleSize, l.TupleNum
	h.Write([]byte(id))
	hID := hex.EncodeToString(h.Sum(nil))
	var tuples = make([]string, tupNum+1)
	for i := 0; i < tupNum; i++ {
		tuples[i] = hID[i*tupSize : (i+1)*tupSize]
	}
	encID := percentEncode(id)
	if len(encID) > 100 {
		encID = encID[:100] + "-" + hID
	}
	tuples[tupNum] = encID
	return strings.Join(tuples, "/"), nil
}

func percentEncode(in string) string {
	shouldEscape := func(c byte) bool {
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '_' {
			return false
		}
		return true
	}
	var numEscape int
	for i := 0; i < len(in); i++ {
		if shouldEscape(in[i]) {
			numEscape++
		}
	}
	if numEscape == 0 {
		return in
	}
	out := make([]byte, len(in)+2*numEscape)
	j := 0
	for i := 0; i < len(in); i++ {
		if shouldEscape(in[i]) {
			out[j] = '%'
			out[j+1] = lowerhex[in[i]>>4]
			out[j+2] = lowerhex[in[i]&15]
			j += 3
			continue
		}
		out[j] = in[i]
		j++
	}
	return string(out)
}
