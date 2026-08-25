package mock_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

const bucket = "call-log-test"

// TestCallLog pins the behavior later tests assert request shape with: every
// served request appears in the log, in order, naming the keys it carried.
func TestCallLog(t *testing.T) {
	ctx := context.Background()
	api := mock.New(bucket, &mock.Object{Key: "a.txt", Body: []byte("a")})

	be.Equal(t, 0, len(api.Calls()))

	_, err := api.PutObject(ctx, &s3v2.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("b.txt"),
		Body:   strings.NewReader("b"),
	})
	be.NilErr(t, err)
	_, err = api.HeadObject(ctx, &s3v2.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("a.txt"),
	})
	be.NilErr(t, err)
	_, err = api.DeleteObject(ctx, &s3v2.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("a.txt"),
	})
	be.NilErr(t, err)

	// Order is request order, across operations.
	calls := api.Calls()
	be.Equal(t, 3, len(calls))
	be.Equal(t, "PutObject", calls[0].Op)
	be.AllEqual(t, []string{"b.txt"}, calls[0].Keys)
	be.Equal(t, "HeadObject", calls[1].Op)
	be.Equal(t, "DeleteObject", calls[2].Op)

	be.Equal(t, 1, api.CallCount("HeadObject"))
	be.Equal(t, 0, api.CallCount("GetObject"))
	be.AllEqual(t, []string{"a.txt"}, api.KeysFor("DeleteObject"))
	batches := api.KeyBatchesFor("DeleteObject")
	be.Equal(t, 1, len(batches))
	be.AllEqual(t, []string{"a.txt"}, batches[0])

	// A failed request is still a request: it is logged even though the mock
	// rejected it.
	_, err = api.HeadObject(ctx, &s3v2.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("gone.txt"),
	})
	be.True(t, err != nil)
	be.AllEqual(t, []string{"a.txt", "gone.txt"}, api.KeysFor("HeadObject"))

	api.ResetCalls()
	be.Equal(t, 0, len(api.Calls()))
}

// TestCallLog_ListPrefix pins how a listing is recorded: by the prefix it
// asked for, with a bucket-wide listing naming no key at all — recording ""
// would make KeysFor report a key that was never sent.
func TestCallLog_ListPrefix(t *testing.T) {
	ctx := context.Background()
	api := mock.New(bucket, &mock.Object{Key: "dir/a.txt", Body: []byte("a")})

	_, err := api.ListObjectsV2(ctx, &s3v2.ListObjectsV2Input{Bucket: aws.String(bucket)})
	be.NilErr(t, err)
	_, err = api.ListObjectsV2(ctx, &s3v2.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("dir/"),
	})
	be.NilErr(t, err)

	be.Equal(t, 2, api.CallCount("ListObjectsV2"))
	be.AllEqual(t, []string{"dir/"}, api.KeysFor("ListObjectsV2"))
	// One slice per request, so the bucket-wide listing stays visible as a
	// call that named nothing rather than disappearing into the flat view.
	batches := api.KeyBatchesFor("ListObjectsV2")
	be.Equal(t, 2, len(batches))
	be.Equal(t, 0, len(batches[0]))
	be.AllEqual(t, []string{"dir/"}, batches[1])
}

// TestDeleteRemovesObject pins the mock's delete fidelity: a deleted key stops
// answering requests, so a test can assert what the bucket holds rather than
// only which requests were sent.
func TestDeleteRemovesObject(t *testing.T) {
	ctx := context.Background()
	api := mock.New(bucket, &mock.Object{Key: "a.txt", Body: []byte("a")})

	_, err := api.DeleteObject(ctx, &s3v2.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("a.txt"),
	})
	be.NilErr(t, err)
	be.True(t, api.Deleted["a.txt"])

	_, err = api.HeadObject(ctx, &s3v2.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("a.txt"),
	})
	be.True(t, err != nil)
	list, err := api.ListObjectsV2(ctx, &s3v2.ListObjectsV2Input{Bucket: aws.String(bucket)})
	be.NilErr(t, err)
	be.Equal(t, 0, len(list.Contents))
}

// TestPutObjectMaterializes pins the other half of that fidelity: an uploaded
// object is readable afterwards, so the mock round-trips a write.
func TestPutObjectMaterializes(t *testing.T) {
	ctx := context.Background()
	api := mock.New(bucket)

	_, err := api.PutObject(ctx, &s3v2.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("new.txt"),
		Body:   strings.NewReader("hello"),
	})
	be.NilErr(t, err)

	head, err := api.HeadObject(ctx, &s3v2.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String("new.txt"),
	})
	be.NilErr(t, err)
	be.Equal(t, int64(len("hello")), aws.ToInt64(head.ContentLength))

	list, err := api.ListObjectsV2(ctx, &s3v2.ListObjectsV2Input{Bucket: aws.String(bucket)})
	be.NilErr(t, err)
	be.Equal(t, 1, len(list.Contents))
	be.Equal(t, "new.txt", aws.ToString(list.Contents[0].Key))
	be.Equal(t, int64(len("hello")), aws.ToInt64(list.Contents[0].Size))
}
