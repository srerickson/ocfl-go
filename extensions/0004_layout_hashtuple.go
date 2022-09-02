package extensions

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/srerickson/ocfl/digest"
)

const Ext0004 = "0004-hashed-n-tuple-storage-layout"

// Extension 0004-hashed-n-tuple-storage-layout
type LayoutHashTuple struct {
	ExtensionName   string      `json:"extensionName"`
	DigestAlgorithm *digest.Alg `json:"digestAlgorithm"`
	TupleSize       int         `json:"tupleSize"`
	TupleNum        int         `json:"numberOfTuples"`
	Short           bool        `json:"shortObjectRoot"`
}

var _ Layout = (*LayoutHashTuple)(nil)
var _ Extension = (*LayoutHashTuple)(nil)

func NewLayoutHashTuple() *LayoutHashTuple {
	return &LayoutHashTuple{
		ExtensionName:   Ext0004,
		DigestAlgorithm: &digest.SHA256,
		TupleSize:       3,
		TupleNum:        3,
		Short:           false,
	}
}

// Name returs the registered name
func (l *LayoutHashTuple) Name() string {
	return Ext0004
}

func (conf *LayoutHashTuple) NewFunc() (LayoutFunc, error) {
	if conf.ExtensionName != conf.Name() {
		return nil, fmt.Errorf("%s: unexpected extensionName %s", conf.Name(), conf.ExtensionName)
	}
	if conf.DigestAlgorithm == nil {
		conf.DigestAlgorithm = &digest.SHA256
	}
	tupSize, tupNum := conf.TupleSize, conf.TupleNum
	if tupSize == 0 && tupNum != 0 {
		return nil, fmt.Errorf("%s: numberOfTuples must be 0", conf.Name())
	}
	if tupNum == 0 && tupSize != 0 {
		return nil, fmt.Errorf("%s: tupleSize must be 0", conf.Name())
	}
	return func(id string) (string, error) {
		h := conf.DigestAlgorithm.New()
		h.Write([]byte(id))
		hID := hex.EncodeToString(h.Sum(nil))
		if tupSize*(tupNum) > len(hID) {
			err := fmt.Errorf("%s: product of tupleSize and numberOfTuples is more then hash length", conf.Name())
			return "", err
		}
		var tuples = make([]string, tupNum+1)
		for i := 0; i < tupNum; i++ {
			tuples[i] = hID[i*tupSize : (i+1)*tupSize]
		}
		if conf.Short {
			tuples[tupNum] = hID[tupNum*tupSize:]
		} else {
			tuples[tupNum] = hID
		}
		return strings.Join(tuples, "/"), nil
	}, nil
}
