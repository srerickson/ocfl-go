package s3

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/carlmjohnson/be"
)

// TestErrIsNotExist_SmithyResponse404 covers the HeadObject-on-real-S3 shape:
// a *smithyhttp.ResponseError with HTTP status 404 wrapping a failed error
// deserialization (HEAD responses have no body to deserialize).
func TestErrIsNotExist_SmithyResponse404(t *testing.T) {
	err := &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusNotFound}},
		Err: &smithy.DeserializationError{
			Err: fmt.Errorf("no response body to deserialize into error shape"),
		},
	}
	be.True(t, errIsNotExist(err))
}

// TestErrIsNotExist_ResponseErrorNon404 ensures other HTTP status codes (e.g.
// 500, 403) are not classified as not-exist by the smithy response check.
func TestErrIsNotExist_ResponseErrorNon404(t *testing.T) {
	for _, code := range []int{http.StatusInternalServerError, http.StatusForbidden, http.StatusUnauthorized} {
		err := &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: code}},
			Err:      fmt.Errorf("some error"),
		}
		be.False(t, errIsNotExist(err))
	}
}

// TestErrIsNotExist_GenericAPIError covers S3-compatible stores (e.g. MinIO)
// that return a HEAD 404 with an XML error body whose code HeadObject's
// deserializer does not map to a typed shape; the SDK falls back to a
// *smithy.GenericAPIError carrying the code.
func TestErrIsNotExist_GenericAPIError(t *testing.T) {
	for _, code := range []string{"NotFound", "NoSuchKey"} {
		be.True(t, errIsNotExist(&smithy.GenericAPIError{Code: code, Message: "m"}))
	}
	// unrelated codes must not be classified as not-exist
	for _, code := range []string{"InternalError", "AccessDenied", "NoSuchBucket"} {
		be.False(t, errIsNotExist(&smithy.GenericAPIError{Code: code, Message: "m"}))
	}
}

// TestErrIsNotExist_Wrapped verifies the checks work through error wrapping
// chains, as produced by the SDK middleware and callers like fs.PathError.
func TestErrIsNotExist_Wrapped(t *testing.T) {
	inner := &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusNotFound}},
		Err:      fmt.Errorf("no body"),
	}
	be.True(t, errIsNotExist(fmt.Errorf("outer: %w", inner)))
	be.False(t, errIsNotExist(nil))
}
