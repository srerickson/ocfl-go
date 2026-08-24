package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// recordingUploader is a manager.UploadAPIClient that records every
// PutObjectInput (including ContentLength and the raw uploaded bytes). It
// fails the test if the uploader ever falls into the multipart path, which
// would mean the bodies in these tests grew past the part size and the
// assertions no longer exercise the single-PutObject branch.
type recordingUploader struct {
	mu   sync.Mutex
	puts []*putCall
}

type putCall struct {
	contentLength *int64
	body          []byte
}

func (u *recordingUploader) PutObject(_ context.Context, in *s3v2.PutObjectInput, _ ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.puts = append(u.puts, &putCall{contentLength: in.ContentLength, body: body})
	return &s3v2.PutObjectOutput{ETag: aws.String("etag")}, nil
}

func (u *recordingUploader) CreateMultipartUpload(context.Context, *s3v2.CreateMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
	return nil, errors.New("test client: unexpected multipart upload")
}

func (u *recordingUploader) UploadPart(context.Context, *s3v2.UploadPartInput, ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
	return nil, errors.New("test client: unexpected UploadPart")
}

func (u *recordingUploader) CompleteMultipartUpload(context.Context, *s3v2.CompleteMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	return nil, errors.New("test client: unexpected CompleteMultipartUpload")
}

func (u *recordingUploader) AbortMultipartUpload(context.Context, *s3v2.AbortMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	return nil, errors.New("test client: unexpected AbortMultipartUpload")
}

func (u *recordingUploader) last() *putCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.puts) == 0 {
		return nil
	}
	return u.puts[len(u.puts)-1]
}

// seekSpy wraps an io.ReadSeeker and records the (offset, whence) arguments of
// every Seek call, so tests can assert exactly how write() sniffs the length
// and that it restores the original position.
type seekSpy struct {
	r      io.ReadSeeker
	seeks  []seekCall
	failAt int // if >= 0, fail the seek at this index (0-based); -1 = never
}

type seekCall struct {
	offset int64
	whence int
}

func (s *seekSpy) Seek(offset int64, whence int) (int64, error) {
	if s.failAt >= 0 && s.failAt == len(s.seeks) {
		return 0, errors.New("seek failed (injected)")
	}
	s.seeks = append(s.seeks, seekCall{offset, whence})
	return s.r.Seek(offset, whence)
}

func (s *seekSpy) Read(p []byte) (int, error) { return s.r.Read(p) }

// newSeekSpy creates a seekSpy that never fails its seeks.
func newSeekSpy(r io.ReadSeeker) *seekSpy {
	return &seekSpy{r: r, failAt: -1}
}

// nonSeekReader is an io.Reader that does not implement io.Seeker (Read only,
// no Seek method).
type nonSeekReader struct {
	r io.Reader
}

func (n nonSeekReader) Read(p []byte) (int, error) { return n.r.Read(p) }

func newTestUploader(t *testing.T, rec *recordingUploader) *manager.Uploader {
	t.Helper()
	return manager.NewUploader(rec)
}

func TestWriteContentLengthSniffing(t *testing.T) {
	const (
		bucket = "test-bucket"
		key    = "content-length-key"
	)
	content := []byte("the quick brown fox jumps over the lazy dog")

	tests := []struct {
		name     string
		reader   func(t *testing.T) (io.Reader, func(), int64)
		wantCL   *int64
		wantBody []byte
	}{
		{
			// A plain *os.File is not an fs.File (its Stat returns
			// os.FileInfo, not fs.FileInfo), so this exercises the
			// generic io.Seeker path.
			name: "*os.File",
			reader: func(t *testing.T) (io.Reader, func(), int64) {
				t.Helper()
				path := filepath.Join(t.TempDir(), "payload.bin")
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
				f, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				return f, func() { f.Close() }, int64(len(content))
			},
			wantCL:   aws.Int64(int64(len(content))),
			wantBody: content,
		},
		{
			// *strings.Reader implements io.Seeker but is not fs.File,
			// *bytes.Reader, or *io.LimitedReader.
			name: "*strings.Reader",
			reader: func(t *testing.T) (io.Reader, func(), int64) {
				t.Helper()
				return strings.NewReader(string(content)), func() {}, int64(len(content))
			},
			wantCL:   aws.Int64(int64(len(content))),
			wantBody: content,
		},
		{
			// Regression: *bytes.Reader must keep its existing case.
			name: "*bytes.Reader",
			reader: func(t *testing.T) (io.Reader, func(), int64) {
				t.Helper()
				return bytes.NewReader(content), func() {}, int64(len(content))
			},
			wantCL:   aws.Int64(int64(len(content))),
			wantBody: content,
		},
		{
			// Regression: *io.LimitedReader must keep its existing case.
			name: "*io.LimitedReader",
			reader: func(t *testing.T) (io.Reader, func(), int64) {
				t.Helper()
				limit := int64(5)
				return &io.LimitedReader{R: bytes.NewReader(content), N: limit}, func() {}, limit
			},
			wantCL:   aws.Int64(5),
			wantBody: content[:5],
		},
		{
			// A non-seekable reader falls back to nil ContentLength.
			name: "non-seekable",
			reader: func(t *testing.T) (io.Reader, func(), int64) {
				t.Helper()
				return nonSeekReader{r: bytes.NewReader(content)}, func() {}, int64(len(content))
			},
			wantCL:   nil,
			wantBody: content,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingUploader{}
			up := newTestUploader(t, rec)
			r, cleanup, _ := tt.reader(t)
			defer cleanup()
			if _, err := write(context.Background(), up, bucket, key, r); err != nil {
				t.Fatalf("write returned error: %v", err)
			}
			call := rec.last()
			if call == nil {
				t.Fatal("no PutObject call recorded")
			}
			gotCL := call.contentLength
			switch {
			case tt.wantCL == nil && gotCL != nil:
				t.Errorf("ContentLength = %d, want nil", *gotCL)
			case tt.wantCL != nil && gotCL == nil:
				t.Error("ContentLength = nil, want non-nil")
			case tt.wantCL != nil && *gotCL != *tt.wantCL:
				t.Errorf("ContentLength = %d, want %d", *gotCL, *tt.wantCL)
			}
			if !bytes.Equal(call.body, tt.wantBody) {
				t.Errorf("uploaded body = %q, want %q", call.body, tt.wantBody)
			}
		})
	}
}

// TestWriteContentLengthRestoresPosition verifies that sniffing seeks to the
// end and restores the original position, and that the uploaded content is
// read from the restored position rather than from the end of the reader.
func TestWriteContentLengthRestoresPosition(t *testing.T) {
	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	spy := newSeekSpy(strings.NewReader("0123456789"))
	// Position the reader mid-stream before the write.
	if _, err := spy.Seek(4, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	spy.seeks = nil // forget the setup seek
	if _, err := write(context.Background(), up, "bucket", "key", spy); err != nil {
		t.Fatalf("write returned error: %v", err)
	}
	// Sniffing must be: remember current position, seek to end, restore.
	wantSeeks := []seekCall{
		{offset: 0, whence: io.SeekCurrent},
		{offset: 0, whence: io.SeekEnd},
		{offset: 4, whence: io.SeekStart},
	}
	if len(spy.seeks) != len(wantSeeks) {
		t.Fatalf("Seek calls = %+v, want %+v", spy.seeks, wantSeeks)
	}
	for i, want := range wantSeeks {
		if spy.seeks[i] != want {
			t.Errorf("Seek call %d = %+v, want %+v", i, spy.seeks[i], want)
		}
	}
	call := rec.last()
	if call == nil {
		t.Fatal("no PutObject call recorded")
	}
	// The uploader must read from the restored position: the suffix
	// starting at offset 4, not the empty remainder at the end.
	if want := []byte("456789"); !bytes.Equal(call.body, want) {
		t.Errorf("uploaded body = %q, want %q", call.body, want)
	}
	if call.contentLength == nil || *call.contentLength != 10 {
		t.Errorf("ContentLength = %v, want 10", call.contentLength)
	}
}

// TestWriteContentLengthSeekFailure verifies that a seekable reader whose
// seeks fail falls back to nil ContentLength instead of erroring or reporting
// a wrong length.
func TestWriteContentLengthSeekFailure(t *testing.T) {
	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	spy := &seekSpy{r: strings.NewReader("0123456789"), failAt: 0}
	if _, err := write(context.Background(), up, "bucket", "key", spy); err != nil {
		t.Fatalf("write returned error: %v", err)
	}
	call := rec.last()
	if call == nil {
		t.Fatal("no PutObject call recorded")
	}
	if call.contentLength != nil {
		t.Errorf("ContentLength = %d, want nil", *call.contentLength)
	}
	// The reader was never disturbed when sniffing failed.
	if !bytes.Equal(call.body, []byte("0123456789")) {
		t.Errorf("uploaded body = %q, want full content", call.body)
	}
}

// TestWriteContentLengthExplicitOption verifies that a caller-provided
// ContentLength option wins over auto-detection (existing behavior).
func TestWriteContentLengthExplicitOption(t *testing.T) {
	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	explicit := aws.Int64(1234)
	r := strings.NewReader("payload")
	if _, err := write(context.Background(), up, "bucket", "key", r, func(in *s3v2.PutObjectInput) {
		in.ContentLength = explicit
	}); err != nil {
		t.Fatalf("write returned error: %v", err)
	}
	call := rec.last()
	if call == nil {
		t.Fatal("no PutObject call recorded")
	}
	if call.contentLength == nil || *call.contentLength != *explicit {
		t.Errorf("ContentLength = %v, want %d", call.contentLength, *explicit)
	}
}

// TestWriteContentLengthInvalidKey is a sanity check that write still
// validates the key before sniffing (so the sniffing code sits after the
// existing validation).
func TestWriteContentLengthInvalidKey(t *testing.T) {
	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	if _, err := write(context.Background(), up, "bucket", "../escape", strings.NewReader("x")); err == nil {
		t.Fatal("write with invalid key returned nil error")
	}
	if rec.last() != nil {
		t.Error("PutObject called with an invalid key")
	}
}
