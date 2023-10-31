package extension

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ext0004         = "0004-hashed-n-tuple-storage-layout"
	shortObjectRoot = "shortObjectRoot"
)

// LayoutHashTuple implements 0004-hashed-n-tuple-storage-layout
type LayoutHashTuple struct {
	DigestAlgorithm string `json:"digestAlgorithm"`
	TupleSize       int    `json:"tupleSize"`
	TupleNum        int    `json:"numberOfTuples"`
	Short           bool   `json:"shortObjectRoot"`
}

var _ (Layout) = (*LayoutHashTuple)(nil)
var _ (Extension) = (*LayoutHashTuple)(nil)

// Ext0004 returns a new instance of 0004-hashed-n-tuple-storage-layout
func Ext0004() Extension {
	return &LayoutHashTuple{
		DigestAlgorithm: `sha256`,
		TupleSize:       3,
		TupleNum:        3,
		Short:           false,
	}
}

func (l LayoutHashTuple) Resolve(id string) (string, error) {
	h := getAlg(l.DigestAlgorithm)
	if h == nil {
		return "", fmt.Errorf("unknown digest algorithm: %q", l.DigestAlgorithm)
	}
	if l.TupleSize == 0 && l.TupleNum != 0 {
		return "", errors.New(numberOfTuples + " must be 0 if " + tupleSize + " is 0")
	}
	if l.TupleNum == 0 && l.TupleSize != 0 {
		return "", errors.New(tupleSize + " must be 0 if " + numberOfTuples + " is 0")
	}
	h.Write([]byte(id))
	hID := hex.EncodeToString(h.Sum(nil))
	tupSize, tupNum := l.TupleSize, l.TupleNum
	if tupSize*(tupNum) > len(hID) {
		err := errors.New("product of tupleSize and numberOfTuples is more then hash length for " + l.DigestAlgorithm)
		return "", err
	}
	var tuples = make([]string, tupNum+1)
	for i := 0; i < tupNum; i++ {
		tuples[i] = hID[i*tupSize : (i+1)*tupSize]
	}
	if l.Short {
		tuples[tupNum] = hID[tupNum*tupSize:]
	} else {
		tuples[tupNum] = hID
	}
	return strings.Join(tuples, "/"), nil
}

func (l LayoutHashTuple) Name() string { return ext0004 }

func (l LayoutHashTuple) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		extensionName:   ext0004,
		digestAlgorithm: l.DigestAlgorithm,
		tupleSize:       l.TupleSize,
		numberOfTuples:  l.TupleNum,
		shortObjectRoot: l.Short,
	})
}
