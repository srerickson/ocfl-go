package s3_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/carlmjohnson/be"

	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

// TestMultiCopy_ConcurrentReuse pins that one MultiCopier receiver may be
// shared across concurrent Copy calls. Copy resolves PartSize and Concurrency
// into locals and never writes them back to the receiver; defaulting a zero
// knob by assigning to the receiver would both race and silently rewrite a
// caller's configuration.
//
// Two rounds cover both knob regimes:
//   - zeroKnobs leaves PartSize and Concurrency at their zero values, the
//     branch that has to apply defaults (32 MiB / 6). The post-condition is
//     that the receiver's fields are still zero afterwards.
//   - explicitKnobs configures non-default values and asserts they survive
//     the concurrent copies untouched, so the no-write rule holds for a
//     caller who has set real values too.
//
// Every goroutine copies the same source to its own destination through the
// shared receiver; all copies must succeed and report the full source size,
// and the multipart upload must complete (never abort).
func TestMultiCopy_ConcurrentReuse(t *testing.T) {
	const (
		src   = "big-src"
		calls = 8
	)
	body := mock.RandBytes(13 * megabyte) // 3 parts at partSize, 1 at the 32 MiB default
	rounds := []struct {
		name            string
		partSize        int64
		concurrency     int
		wantPartSize    int64
		wantConcurrency int
	}{
		{name: "zeroKnobs", partSize: 0, concurrency: 0, wantPartSize: 0, wantConcurrency: 0},
		{name: "explicitKnobs", partSize: partSize, concurrency: 2, wantPartSize: partSize, wantConcurrency: 2},
	}
	for _, tc := range rounds {
		t.Run(tc.name, func(t *testing.T) {
			api := mock.New(bucket, &mock.Object{Key: src, Body: body})
			copier := s3.NewMultiCopier(api, func(mc *s3.MultiCopier) {
				mc.PartSize = tc.partSize
				mc.Concurrency = tc.concurrency
			})

			// Release all goroutines at once so the per-call reads of the
			// receiver's knobs overlap as much as possible.
			start := make(chan struct{})
			errs := make([]error, calls)
			sizes := make([]int64, calls)
			var wg sync.WaitGroup
			for i := range calls {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					size, err := copier.Copy(context.Background(), bucket, fmt.Sprintf("dst-%d", i), src)
					errs[i] = err
					sizes[i] = size
				}(i)
			}
			close(start)
			wg.Wait()

			for i := range calls {
				if err := errs[i]; err != nil {
					t.Errorf("copy %d failed: %v", i, err)
				}
				if sizes[i] != int64(len(body)) {
					t.Errorf("copy %d reported size %d, want %d", i, sizes[i], len(body))
				}
			}
			// every destination must have been completed by the mock
			if len(api.UpdatedETags) != calls {
				t.Errorf("expected %d completed destinations, got %d", calls, len(api.UpdatedETags))
			}
			be.True(t, api.MPUComplete)
			be.False(t, api.MPUAborted)
			// concurrent Copy calls must never write to the shared receiver
			if copier.PartSize != tc.wantPartSize {
				t.Errorf("Copy modified receiver PartSize: got %d, want %d", copier.PartSize, tc.wantPartSize)
			}
			if copier.Concurrency != tc.wantConcurrency {
				t.Errorf("Copy modified receiver Concurrency: got %d, want %d", copier.Concurrency, tc.wantConcurrency)
			}
		})
	}
}
