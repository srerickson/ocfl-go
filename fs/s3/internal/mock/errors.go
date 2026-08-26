package mock

import (
	"errors"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// errMissingKey is the mock's internal sentinel for "the bucket does not hold
// this key". It never reaches a caller: each handler converts it into the
// error shape the configured NotFoundStyle produces, because the shape a
// request gets back depends on which operation asked (a HEAD has no body to
// deserialize a typed error from; a GET does) and getObjectLocked does not
// know which one that was.
var errMissingKey = errors.New("mock: no such key")

// NotFoundStyle selects the error shape the mock returns for a request naming
// a key the bucket does not hold. Two families of store are worth pinning,
// because the s3 backend's not-exist mapping has to recognize both.
type NotFoundStyle int

const (
	// NotFoundStyleAWS is what the SDK hands back from a real S3 endpoint:
	// the operation's own typed error (*types.NotFound for HeadObject,
	// *types.NoSuchKey for GetObject and CopyObject), wrapped in the
	// *smithyhttp.ResponseError that carries the 404 response, wrapped in
	// turn by the AWS transport's ResponseError that carries the request
	// ID. A caller sees all three: errors.As finds the typed error, and
	// the status and request ID stay reachable on the way to it.
	//
	// This is the default: a mock that lies about the common case is worse
	// than no mock.
	NotFoundStyleAWS NotFoundStyle = iota

	// NotFoundStyleGeneric is what an S3-compatible store produces when the
	// SDK cannot resolve a typed error for the code it sent back: a
	// *smithy.GenericAPIError carrying the code as a string, still inside
	// the 404 ResponseError. Nothing about it is deserializable into
	// *types.NotFound or *types.NoSuchKey, so a not-exist check written
	// against the typed errors alone does not recognize it.
	NotFoundStyleGeneric
)

// notFound returns the error a request for a missing key gets back, in the
// shape m is configured for. typed is the SDK error type the operation
// declares for a missing key -- it is what NotFoundStyleAWS wraps, and what
// NotFoundStyleGeneric replaces with a code-carrying generic error.
func (m *S3API) notFound(typed smithy.APIError) error {
	var inner error = typed
	if m.notFoundStyle == NotFoundStyleGeneric {
		inner = &smithy.GenericAPIError{
			Code:    typed.ErrorCode(),
			Message: typed.ErrorMessage(),
		}
	}
	return &awshttp.ResponseError{
		ResponseError: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{
				Response: &http.Response{
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Header:     http.Header{},
				},
			},
			Err: inner,
		},
		RequestID: mockRequestID,
	}
}

// mockRequestID stands in for the request ID a real endpoint returns. Tests
// assert on it to show that the s3 backend's not-exist mapping keeps the
// cause reachable instead of replacing it.
const mockRequestID = "MOCKREQUESTID0001"

// RequestID returns the request ID the mock stamps on its error responses.
func RequestID() string { return mockRequestID }

// noSuchKeyErr converts getObject's missing-key sentinel into the shape an
// operation with a response body returns -- GetObject, CopyObject and
// UploadPartCopy all deserialize *types.NoSuchKey from one. Any other error
// passes through untouched.
func (m *S3API) noSuchKeyErr(err error) error {
	if !errors.Is(err, errMissingKey) {
		return err
	}
	return m.notFound(&types.NoSuchKey{
		Message: aws.String("The specified key does not exist."),
	})
}
