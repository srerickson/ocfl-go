package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// conditionalPutUploader is a manager.UploadAPIClient whose PutObject returns
// a canned error. The multipart methods fail loudly so the test proves the
// single-PutObject branch is the one being exercised.
type conditionalPutUploader struct {
	putErr error
}

func (u *conditionalPutUploader) PutObject(_ context.Context, _ *s3v2.PutObjectInput, _ ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
	return nil, u.putErr
}

func (u *conditionalPutUploader) CreateMultipartUpload(context.Context, *s3v2.CreateMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
	return nil, errors.New("test client: unexpected multipart upload")
}

func (u *conditionalPutUploader) UploadPart(context.Context, *s3v2.UploadPartInput, ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
	return nil, errors.New("test client: unexpected UploadPart")
}

func (u *conditionalPutUploader) CompleteMultipartUpload(context.Context, *s3v2.CompleteMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	return nil, errors.New("test client: unexpected CompleteMultipartUpload")
}

func (u *conditionalPutUploader) AbortMultipartUpload(context.Context, *s3v2.AbortMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	return nil, errors.New("test client: unexpected AbortMultipartUpload")
}

// TestWriteConditionalPutErrorMapping pins the error mapping for a conditional
// PUT rejected by the store (If-None-Match: "*" on an existing key -> 412
// PreconditionFailed) without needing a live store. It is the unit-level
// counterpart of the integration test TestWriteWithOptions (fs_test.go): the
// error returned by write() must remain a *fs.PathError for the fs API
// contract, and it must satisfy errors.As into smithy.APIError with the
// service's error code, regardless of how many SDK layers wrap it.
func TestWriteConditionalPutErrorMapping(t *testing.T) {
	apiErr := &smithy.GenericAPIError{
		Code:    "PreconditionFailed",
		Message: "At least one of the pre-conditions you specified did not hold",
	}
	// The SDK surfaces a rejected conditional PUT as an operation error
	// wrapping the deserialized API error; some stores and SDK paths wrap
	// further (retry errors, response errors). Both shapes must map.
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "operation error wrapping API error",
			err: &smithy.OperationError{
				ServiceID:     "S3",
				OperationName: "PutObject",
				Err:           apiErr,
			},
		},
		{
			name: "bare API error",
			err:  apiErr,
		},
		{
			name: "double-wrapped API error",
			err: &smithy.OperationError{
				ServiceID:     "S3",
				OperationName: "PutObject",
				Err: &smithy.OperationError{
					ServiceID:     "S3",
					OperationName: "PutObject",
					Err:           apiErr,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := manager.NewUploader(&conditionalPutUploader{putErr: tt.err})
			match := "*"
			n, err := write(context.Background(), up, "bucket", "key", strings.NewReader("content"), func(in *s3v2.PutObjectInput) {
				in.IfNoneMatch = &match
			})
			if err == nil {
				t.Fatal("write() returned nil error for rejected conditional PUT")
			}
			if n != 0 {
				t.Errorf("write() returned %d bytes, want 0 on error", n)
			}
			// fs-layer contract: the error is a *fs.PathError for the
			// write operation on the key.
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("errors.As(err, &*fs.PathError) = false, err = %T %v", err, err)
			}
			if pathErr.Op != "write" || pathErr.Path != "key" {
				t.Errorf("PathError = {%q %q}, want {%q %q}", pathErr.Op, pathErr.Path, "write", "key")
			}
			// The API error must be reachable through the wrapping.
			var apiGot smithy.APIError
			if !errors.As(err, &apiGot) {
				t.Fatalf("errors.As(err, &smithy.APIError) = false, err = %T %v", err, err)
			}
			if got := apiGot.ErrorCode(); got != "PreconditionFailed" {
				t.Errorf("ErrorCode() = %q, want %q", got, "PreconditionFailed")
			}
		})
	}
}

// TestWriteExhaustedReaderContentLength pins the regression behind
// TestWriteWithOptions (fs_test.go): writing with an exhausted seekable reader
// (for example, the same strings.Reader or bytes.Reader reused for a second
// conditional write) must not declare the stream's original ContentLength --
// the request would break on the wire ("net/http: ContentLength=7 with Body
// length 0") and never reach the store, so a rejected conditional PUT could
// not surface its 412 PreconditionFailed API error. The remaining length is 0,
// so ContentLength must be 0 and the body empty, keeping the request
// well-formed. The bytes.Reader variant is the regression for the dedicated
// `case *bytes.Reader` that used Size() (the total slice length) even for a
// fully-consumed reader.
func TestWriteExhaustedReaderContentLength(t *testing.T) {
	const content = "content"
	tests := []struct {
		name   string
		reader func() io.Reader
	}{
		{
			name: "strings.Reader",
			reader: func() io.Reader {
				return strings.NewReader(content)
			},
		},
		{
			name: "bytes.Reader",
			reader: func() io.Reader {
				return bytes.NewReader([]byte(content))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingUploader{}
			up := newTestUploader(t, rec)
			r := tt.reader()
			// Fully consume the reader, as a first write would.
			if _, err := io.Copy(io.Discard, r); err != nil {
				t.Fatal(err)
			}
			if _, err := write(context.Background(), up, "bucket", "key", r); err != nil {
				t.Fatalf("write returned error: %v", err)
			}
			call := rec.last()
			if call == nil {
				t.Fatal("no PutObject call recorded")
			}
			if call.contentLength == nil || *call.contentLength != 0 {
				t.Errorf("ContentLength = %v, want 0 (remaining bytes of an exhausted reader)", call.contentLength)
			}
			if len(call.body) != 0 {
				t.Errorf("uploaded body = %q, want empty", call.body)
			}
		})
	}
}
