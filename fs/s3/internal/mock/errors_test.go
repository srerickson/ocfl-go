package mock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/carlmjohnson/be"

	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

const errBucket = "mock-errors-test"

// TestDeleteObjectIsIdempotent pins the half of the DeleteObject contract that
// the s3 backend's Remove has to work around: a real endpoint answers 204 for
// a key that was never there and says nothing about which it was. The mock
// used to return NoSuchKey instead, which made the backend's Remove look
// strict under test while returning nil against a real bucket.
func TestDeleteObjectIsIdempotent(t *testing.T) {
	ctx := context.Background()
	api := mock.New(errBucket, &mock.Object{Key: "present.txt"})

	out, err := api.DeleteObject(ctx, &s3v2.DeleteObjectInput{
		Bucket: aws.String(errBucket),
		Key:    aws.String("never-existed.txt"),
	})
	be.NilErr(t, err)
	be.True(t, out != nil)

	// Deleting a key that is there behaves the same way, and the key stops
	// answering HeadObject.
	_, err = api.DeleteObject(ctx, &s3v2.DeleteObjectInput{
		Bucket: aws.String(errBucket),
		Key:    aws.String("present.txt"),
	})
	be.NilErr(t, err)
	_, err = api.HeadObject(ctx, &s3v2.HeadObjectInput{
		Bucket: aws.String(errBucket),
		Key:    aws.String("present.txt"),
	})
	be.True(t, err != nil)

	// Both requests are in the call log, so a test can still tell which
	// keys a delete named even though neither failed.
	be.AllEqual(t, []string{"never-existed.txt", "present.txt"}, api.KeysFor("DeleteObject"))

	// A request naming the wrong bucket still fails: idempotence is about
	// the key, not about accepting anything.
	_, err = api.DeleteObject(ctx, &s3v2.DeleteObjectInput{
		Bucket: aws.String("some-other-bucket"),
		Key:    aws.String("present.txt"),
	})
	be.True(t, err != nil)
}

// TestNotFoundShapes pins the error shapes a request for a missing key comes
// back as. They are what the s3 backend's not-exist mapping is written
// against, so a mock that invents its own shape hides whether that mapping
// works against a real store.
func TestNotFoundShapes(t *testing.T) {
	ctx := context.Background()

	t.Run("aws style", func(t *testing.T) {
		api := mock.New(errBucket)

		// HeadObject: no response body, so a real endpoint's code is
		// derived from the 404 status and deserializes to *types.NotFound.
		_, err := api.HeadObject(ctx, &s3v2.HeadObjectInput{
			Bucket: aws.String(errBucket),
			Key:    aws.String("missing.txt"),
		})
		be.True(t, err != nil)
		var notFound *types.NotFound
		be.True(t, errors.As(err, &notFound))

		// GetObject: the body carries NoSuchKey.
		_, err = api.GetObject(ctx, &s3v2.GetObjectInput{
			Bucket: aws.String(errBucket),
			Key:    aws.String("missing.txt"),
		})
		be.True(t, err != nil)
		var noSuchKey *types.NoSuchKey
		be.True(t, errors.As(err, &noSuchKey))

		// Either way the transport error is reachable, carrying the status
		// and request ID a caller needs to make sense of a failure.
		var respErr *smithyhttp.ResponseError
		be.True(t, errors.As(err, &respErr))
		be.Equal(t, 404, respErr.HTTPStatusCode())
		var reqIDErr interface{ ServiceRequestID() string }
		be.True(t, errors.As(err, &reqIDErr))
		be.Equal(t, mock.RequestID(), reqIDErr.ServiceRequestID())
	})

	t.Run("generic style", func(t *testing.T) {
		api := mock.New(errBucket).WithNotFoundStyle(mock.NotFoundStyleGeneric)

		for _, tc := range []struct {
			op   string
			call func() error
			code string
		}{
			{
				op:   "HeadObject",
				code: "NotFound",
				call: func() error {
					_, err := api.HeadObject(ctx, &s3v2.HeadObjectInput{
						Bucket: aws.String(errBucket),
						Key:    aws.String("missing.txt"),
					})
					return err
				},
			},
			{
				op:   "GetObject",
				code: "NoSuchKey",
				call: func() error {
					_, err := api.GetObject(ctx, &s3v2.GetObjectInput{
						Bucket: aws.String(errBucket),
						Key:    aws.String("missing.txt"),
					})
					return err
				},
			},
		} {
			t.Run(tc.op, func(t *testing.T) {
				err := tc.call()
				be.True(t, err != nil)

				// The code survives as a string on a generic error...
				var apiErr *smithy.GenericAPIError
				be.True(t, errors.As(err, &apiErr))
				be.Equal(t, tc.code, apiErr.ErrorCode())

				// ...and there is nothing typed to find, which is the whole
				// point of pinning this style: a not-exist check written
				// against the typed errors alone does not recognize it.
				var notFound *types.NotFound
				be.False(t, errors.As(err, &notFound))
				var noSuchKey *types.NoSuchKey
				be.False(t, errors.As(err, &noSuchKey))

				// The 404 is still there to be read.
				var respErr *smithyhttp.ResponseError
				be.True(t, errors.As(err, &respErr))
				be.Equal(t, 404, respErr.HTTPStatusCode())
			})
		}
	})
}
