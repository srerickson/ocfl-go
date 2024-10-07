package extension

import (
	"encoding/hex"
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
	Base
	DigestAlgorithm string `json:"digestAlgorithm"`
	TupleSize       int    `json:"tupleSize"`
	TupleNum        int    `json:"numberOfTuples"`
	Short           bool   `json:"shortObjectRoot"`
}

// Ext0004 returns a new instance of 0004-hashed-n-tuple-storage-layout
func Ext0004() Extension {
	return &LayoutHashTuple{
		Base:            Base{ExtensionName: ext0004},
		DigestAlgorithm: `sha256`,
		TupleSize:       3,
		TupleNum:        3,
		Short:           false,
	}
}

func (l LayoutHashTuple) Valid() error {
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

func (l LayoutHashTuple) Resolve(id string) (string, error) {
	if err := l.Valid(); err != nil {
		return "", err
	}
	h := getHash(l.DigestAlgorithm)
	h.Write([]byte(id))
	hID := hex.EncodeToString(h.Sum(nil))
	tupSize, tupNum := l.TupleSize, l.TupleNum
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
