package mock

import (
	"slices"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Call is one request the mock received: the operation name and the object
// keys the request named, in request order. A batch operation records every
// key it carried, which is what lets a test tell one DeleteObjects of 500 keys
// apart from 500 separate DeleteObject calls.
type Call struct {
	Op   string
	Keys []string
}

// callLog is the mock's ordered record of the requests it served.
//
// It exists so tests can assert request shape — which operations ran, in what
// order, carrying which keys — without each one hand-writing a wrapper type
// that embeds *S3API and overrides two methods to append to a slice. Those
// wrappers were identical wherever they appeared, and being per-test they
// could not observe anything a test had not thought to wrap.
//
// The log is guarded by its own mutex rather than the mock's MPU mutex: every
// method records, including the ones that already hold that lock.
type callLog struct {
	mu    sync.Mutex
	calls []Call
}

func (l *callLog) record(op string, keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, Call{Op: op, Keys: keys})
}

// recordKey records a call naming a single, possibly nil, key. A nil key is
// recorded as a call with no keys, so a malformed request still appears in the
// log rather than vanishing from it.
func (l *callLog) recordKey(op string, key *string) {
	if key == nil {
		l.record(op)
		return
	}
	l.record(op, *key)
}

// Calls returns the ordered log of every request the mock has served.
func (m *S3API) Calls() []Call {
	m.log.mu.Lock()
	defer m.log.mu.Unlock()
	return slices.Clone(m.log.calls)
}

// CallsFor returns the calls to op, in order.
func (m *S3API) CallsFor(op string) []Call {
	var out []Call
	for _, c := range m.Calls() {
		if c.Op == op {
			out = append(out, c)
		}
	}
	return out
}

// CallCount returns how many times op was called. Note the distinction from
// KeysFor: three DeleteObject calls and one DeleteObjects carrying three keys
// both name three keys, but only the first has a CallCount of 3.
func (m *S3API) CallCount(op string) int {
	return len(m.CallsFor(op))
}

// KeysFor returns every key named by every call to op, flattened in call
// order. Use CallsFor when the grouping into requests matters.
func (m *S3API) KeysFor(op string) []string {
	var out []string
	for _, c := range m.CallsFor(op) {
		out = append(out, c.Keys...)
	}
	return out
}

// KeyBatchesFor returns the keys named by each call to op, one slice per call.
// This is the batching-sensitive view: a backend that regressed from one
// batched delete to one delete per key returns the same KeysFor and a very
// different KeyBatchesFor.
func (m *S3API) KeyBatchesFor(op string) [][]string {
	out := [][]string{}
	for _, c := range m.CallsFor(op) {
		out = append(out, c.Keys)
	}
	return out
}

// ResetCalls discards the log, for a test that sets up through the same API it
// is about to make assertions on.
func (m *S3API) ResetCalls() {
	m.log.mu.Lock()
	defer m.log.mu.Unlock()
	m.log.calls = nil
}

// deleteObjectsKeys extracts the keys carried by a DeleteObjects request.
func deleteObjectsKeys(objects []types.ObjectIdentifier) []string {
	keys := make([]string, 0, len(objects))
	for _, obj := range objects {
		if obj.Key != nil {
			keys = append(keys, aws.ToString(obj.Key))
		}
	}
	return keys
}
