package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// closeErrBody is an io.ReadCloser whose Close always fails, used to verify
// that Seek logs (rather than silently discarding) body close errors.
type closeErrBody struct {
	io.Reader
}

func (closeErrBody) Close() error { return errors.New("close failed") }

// seekTestLogger returns a debug-level slog.Logger that writes to buf. The
// Level must be slog.LevelDebug or the package's debug-level messages are
// filtered out before reaching the handler.
func seekTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// newTestS3File builds an s3File directly (bypassing openFile) with a body
// already in place and the offset past the start, so a Seek to position 0
// triggers the body-close path.
func newTestS3File(body io.ReadCloser, logger *slog.Logger) *s3File {
	return &s3File{
		ctx:    context.Background(),
		bucket: "test-bucket",
		key:    "test-key",
		body:   body,
		offset: 5,
		info:   &s3v2.HeadObjectOutput{ContentLength: aws.Int64(64)},
		logger: logger,
	}
}

func TestSeekLogsBodyCloseError(t *testing.T) {
	var buf bytes.Buffer
	f := newTestS3File(closeErrBody{Reader: strings.NewReader("0123456789")}, seekTestLogger(&buf))

	pos, err := f.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek returned error: %v", err)
	}
	if pos != 0 {
		t.Fatalf("Seek position = %d, want 0", pos)
	}
	logged := buf.String()
	if !strings.Contains(logged, "s3:seek:close") {
		t.Errorf("close error was not logged (log output: %q)", logged)
	}
	if !strings.Contains(logged, "close failed") {
		t.Errorf("log does not include the close error (log output: %q)", logged)
	}
	if f.body != nil {
		t.Error("Seek did not discard the old body")
	}
}

func TestSeekBodyCloseErrorWithoutLogger(t *testing.T) {
	// Without a configured logger, a failing body close must not panic and
	// Seek must still succeed.
	f := newTestS3File(closeErrBody{Reader: strings.NewReader("0123456789")}, nil)

	pos, err := f.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek returned error: %v", err)
	}
	if pos != 0 {
		t.Fatalf("Seek position = %d, want 0", pos)
	}
	if f.body != nil {
		t.Error("Seek did not discard the old body")
	}
}

func TestSeekSamePositionDoesNotCloseBody(t *testing.T) {
	// Seeking to the current position must not invalidate the body, exactly
	// as before the close-error logging change.
	var buf bytes.Buffer
	f := newTestS3File(closeErrBody{Reader: strings.NewReader("0123456789")}, seekTestLogger(&buf))

	pos, err := f.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek returned error: %v", err)
	}
	if pos != 5 {
		t.Fatalf("Seek position = %d, want 5", pos)
	}
	if f.body == nil {
		t.Error("Seek to the current position closed the body")
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected log output: %q", buf.String())
	}
}

// closeErrAPI is an OpenFileAPI whose GetObject always returns a body whose
// Close fails.
type closeErrAPI struct{}

func (closeErrAPI) HeadObject(context.Context, *s3v2.HeadObjectInput, ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	return &s3v2.HeadObjectOutput{ContentLength: aws.Int64(16)}, nil
}

func (closeErrAPI) GetObject(context.Context, *s3v2.GetObjectInput, ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
	return &s3v2.GetObjectOutput{Body: closeErrBody{Reader: strings.NewReader("0123456789abcdef")}}, nil
}

// TestSeekCloseErrorViaOpenFile exercises the full path: the logger configured
// on the BucketFS must reach the s3File via openFile and be used by Seek when
// closing the old body fails.
func TestSeekCloseErrorViaOpenFile(t *testing.T) {
	var buf bytes.Buffer
	logger := seekTestLogger(&buf)

	file, err := openFile(context.Background(), closeErrAPI{}, "test-bucket", "test-key", logger)
	if err != nil {
		t.Fatalf("openFile returned error: %v", err)
	}
	seeker, ok := file.(io.Seeker)
	if !ok {
		t.Fatal("opened file does not implement io.Seeker")
	}
	readBuf := make([]byte, 16)
	if _, err := file.Read(readBuf); err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek returned error: %v", err)
	}
	logged := buf.String()
	if !strings.Contains(logged, "s3:seek:close") {
		t.Errorf("close error was not logged (log output: %q)", logged)
	}
	if !strings.Contains(logged, "close failed") {
		t.Errorf("log does not include the close error (log output: %q)", logged)
	}
}
