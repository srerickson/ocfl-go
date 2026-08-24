package s3

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// pathErr makes fs.PathError errors
func pathErr(op string, path string, err error) error {
	return &fs.PathError{Op: op, Path: path, Err: err}
}

// notExistErr wraps fs.ErrNotExist around err instead of replacing it, so
// callers can match the missing-file contract with errors.Is(err,
// fs.ErrNotExist) while the original smithy/HTTP error (status code, request
// ID, etc.) remains reachable through the unwrap chain for debugging against
// MinIO and real S3. All not-exist error paths in this package must use it.
func notExistErr(err error) error {
	return fmt.Errorf("%w: %w", fs.ErrNotExist, err)
}

// errIsNotExist reports whether err represents a missing object ("not found")
// error from S3 or an S3-compatible store. Callers use it to map HeadObject
// (and similar) failures to fs.ErrNotExist.
//
// The error shapes the AWS SDK v2 can produce for a missing object depend on
// the service and on whether the error response had a body:
//
//   - Operations whose errors deserialize from a body (e.g. GetObject) return
//     the typed shapes types.NotFound and types.NoSuchKey.
//   - HeadObject against real S3 returns *smithyhttp.ResponseError with HTTP
//     status 404 wrapping the failed error deserialization, because HEAD
//     responses carry no body from which to deserialize an error shape.
//   - Some S3-compatible stores (e.g. MinIO) return a HEAD 404 with an XML
//     body whose code (commonly "NoSuchKey") is not one of the shapes
//     HeadObject's deserializer recognizes (it only maps "NotFound"); in that
//     case the SDK falls back to *smithy.GenericAPIError carrying the code.
func errIsNotExist(err error) bool {
	var notFoundErr *types.NotFound
	if errors.As(err, &notFoundErr) {
		return true
	}
	var noKeyErr *types.NoSuchKey
	if errors.As(err, &noKeyErr) {
		return true
	}
	// HeadObject on a missing object: real S3 returns an HTTP 404 with no
	// body, which surfaces as a generic smithy http response error.
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.Response != nil &&
		respErr.Response.StatusCode == http.StatusNotFound {
		return true
	}
	// S3-compatible stores that include an error body on HEAD 404 (e.g. MinIO
	// with code "NoSuchKey", which HeadObject's deserializer does not map to a
	// typed shape) surface as a generic API error carrying the code.
	var genericErr *smithy.GenericAPIError
	if errors.As(err, &genericErr) {
		switch genericErr.Code {
		case "NotFound", "NoSuchKey":
			return true
		}
	}
	return false
}
