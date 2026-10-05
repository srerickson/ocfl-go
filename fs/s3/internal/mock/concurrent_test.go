package mock_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

// TestMockConcurrentUse drives the mock from many goroutines at once —
// writes, reads, lists and deletes over overlapping keys — the way the s3
// backend's multipart uploader and copier do. Run under -race it pins that
// the mock's state (objects, Deleted, UpdatedETags, MPU flags) is as guarded
// as its call log: unsynchronized map access here is a fatal race, not a
// flaky result.
func TestMockConcurrentUse(t *testing.T) {
	ctx := context.Background()
	api := mock.New(bucket, &mock.Object{Key: "seed.txt", Body: []byte("seed")})

	const workers = 8
	const opsPerWorker = 25
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			key := fmt.Sprintf("worker-%d.txt", w%4) // overlapping keys across workers
			for i := 0; i < opsPerWorker; i++ {
				_, _ = api.PutObject(ctx, &s3v2.PutObjectInput{
					Bucket: aws.String(bucket),
					Key:    aws.String(key),
					Body:   strings.NewReader(fmt.Sprintf("w%d i%d", w, i)),
				})
				_, _ = api.GetObject(ctx, &s3v2.GetObjectInput{
					Bucket: aws.String(bucket),
					Key:    aws.String(key),
				})
				_, _ = api.HeadObject(ctx, &s3v2.HeadObjectInput{
					Bucket: aws.String(bucket),
					Key:    aws.String(key),
				})
				_, _ = api.ListObjectsV2(ctx, &s3v2.ListObjectsV2Input{
					Bucket: aws.String(bucket),
					Prefix: aws.String("worker-"),
				})
				_, _ = api.DeleteObject(ctx, &s3v2.DeleteObjectInput{
					Bucket: aws.String(bucket),
					Key:    aws.String(key),
				})
			}
		}(w)
	}
	wg.Wait()

	// Every worker's ops were served and recorded; state accessors agree with
	// the request log.
	be.Nonzero(t, api.Calls())
	be.True(t, api.CallCount("PutObject") >= workers*opsPerWorker)
	// A final single-threaded round-trip confirms the bucket is coherent
	// after the concurrent run.
	_, err := api.PutObject(ctx, &s3v2.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("final.txt"),
		Body:   strings.NewReader("final"),
	})
	be.NilErr(t, err)
	head, err := api.HeadObject(ctx, &s3v2.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("final.txt"),
	})
	be.NilErr(t, err)
	be.Equal(t, int64(len("final")), aws.ToInt64(head.ContentLength))
	be.Nonzero(t, api.UpdatedETag("final.txt"))
	be.True(t, api.WasDeleted("worker-0.txt") || api.WasDeleted("worker-1.txt") ||
		api.WasDeleted("worker-2.txt") || api.WasDeleted("worker-3.txt"))
}
