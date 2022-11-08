package extensions

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/srerickson/ocfl/digest"
)

const (
	Ext0003  = "0003-hash-and-id-n-tuple-storage-layout"
	lowerhex = "0123456789abcdef"
)

// Extension 0003-hash-and-id-n-tuple-storage-layout
type LayoutHashIDTuple struct {
	ExtensionName   string `json:"extensionName"`
	DigestAlgorithm string `json:"digestAlgorithm"`
	TupleSize       int    `json:"tupleSize"`
	TupleNum        int    `json:"numberOfTuples"`
}

var _ Layout = (*LayoutHashIDTuple)(nil)
var _ Extension = (*LayoutHashIDTuple)(nil)

func NewLayoutHashIDTuple() *LayoutHashIDTuple {
	return &LayoutHashIDTuple{
		ExtensionName:   Ext0003,
		DigestAlgorithm: digest.SHA256id,
		TupleSize:       3,
		TupleNum:        3,
	}
}

func (l *LayoutHashIDTuple) Name() string {
	return Ext0003
}

func (l *LayoutHashIDTuple) NewFunc() (LayoutFunc, error) {
	if l.ExtensionName != l.Name() {
		return nil, fmt.Errorf("%s: unexpected extensionName %s", l.Name(), l.ExtensionName)
	}
	alg, err := digest.Get(l.DigestAlgorithm)
	if err != nil {
		return nil, err
	}
	tupSize, tupNum := l.TupleSize, l.TupleNum
	if tupSize == 0 && tupNum != 0 {
		return nil, fmt.Errorf("%s: numberOfTuples must be 0", l.Name())
	}
	if tupNum == 0 && tupSize != 0 {
		return nil, fmt.Errorf("%s: tupleSize must be 0", l.Name())
	}
	return func(id string) (string, error) {
		h := alg.New()
		h.Write([]byte(id))
		hID := hex.EncodeToString(h.Sum(nil))
		if tupSize*(tupNum) > len(hID) {
			err := fmt.Errorf("%s: product of tupleSize and numberOfTuples is more then hash length", l.Name())
			return "", err
		}
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

	}, nil
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
