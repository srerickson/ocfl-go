package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
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
	r          io.ReadSeeker
	seeks      []seekCall
	failAt     int // if >= 0, fail the seek at this index (0-based); -1 = never
	failOnceAt int // if >= 0, fail the seek at this index exactly once; -1 = never
}

type seekCall struct {
	offset int64
	whence int
}

func (s *seekSpy) Seek(offset int64, whence int) (int64, error) {
	if s.failAt >= 0 && s.failAt == len(s.seeks) {
		return 0, errors.New("seek failed (injected)")
	}
	if s.failOnceAt >= 0 && s.failOnceAt == len(s.seeks) {
		s.failOnceAt = -1 // fail exactly once, then recover
		return 0, errors.New("seek failed (injected once)")
	}
	s.seeks = append(s.seeks, seekCall{offset, whence})
	return s.r.Seek(offset, whence)
}

func (s *seekSpy) Read(p []byte) (int, error) { return s.r.Read(p) }

// newSeekSpy creates a seekSpy that never fails its seeks.
func newSeekSpy(r io.ReadSeeker) *seekSpy {
	return &seekSpy{r: r, failAt: -1, failOnceAt: -1}
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
			// *os.File implements both fs.File and io.Seeker (os.FileInfo
			// is an alias for fs.FileInfo since Go 1.16, so its Stat
			// signature satisfies fs.File). It must take the io.Seeker
			// path so ContentLength is sniffed from the remaining length;
			// at offset 0 that equals the total file size.
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
			// Regression: a partially-consumed *os.File is both fs.File
			// and io.Seeker. The seeker's REMAINING length must win over
			// fs.File.Stat's total size; otherwise ContentLength
			// over-reports and net/http rejects the request.
			name: "*os.File partially consumed",
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
				// advance past the first 10 bytes, leaving content[10:]
				if _, err := io.ReadFull(f, make([]byte, 10)); err != nil {
					t.Fatalf("ReadFull: %v", err)
				}
				return f, func() { f.Close() }, int64(len(content) - 10)
			},
			wantCL:   aws.Int64(int64(len(content) - 10)),
			wantBody: content[10:],
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
			// Regression (4310a03): a partially-consumed *strings.Reader
			// (e.g. one reused after a first Write, as in the integration
			// test TestWriteWithOptions) must report the REMAINING length,
			// not the total size. A ContentLength larger than the body
			// breaks the HTTP request before it reaches S3.
			name: "*strings.Reader partially consumed",
			reader: func(t *testing.T) (io.Reader, func(), int64) {
				t.Helper()
				r := strings.NewReader(string(content))
				// advance past the first 10 bytes, leaving content[10:]
				for i := 0; i < 10; i++ {
					if _, err := r.ReadByte(); err != nil {
						t.Fatalf("ReadByte: %v", err)
					}
				}
				return r, func() {}, int64(len(content) - 10)
			},
			wantCL:   aws.Int64(int64(len(content) - 10)),
			wantBody: content[10:],
		},
		{
			// *bytes.Reader implements io.Seeker, so a fresh reader
			// routes through the seeker sniff and reports the full
			// length (remaining == total at offset 0) — unchanged
			// behavior now that it no longer has its own case.
			name: "*bytes.Reader",
			reader: func(t *testing.T) (io.Reader, func(), int64) {
				t.Helper()
				return bytes.NewReader(content), func() {}, int64(len(content))
			},
			wantCL:   aws.Int64(int64(len(content))),
			wantBody: content,
		},
		{
			// Regression: a partially-consumed *bytes.Reader must
			// report the REMAINING length, not the total size of the
			// underlying slice. The old dedicated `case *bytes.Reader`
			// used Size() (total), which over-declared ContentLength
			// while the body delivered only the tail — net/http rejects
			// such requests on the wire.
			name: "*bytes.Reader partially consumed",
			reader: func(t *testing.T) (io.Reader, func(), int64) {
				t.Helper()
				r := bytes.NewReader(content)
				// advance past the first 10 bytes, leaving content[10:]
				if _, err := io.ReadFull(r, make([]byte, 10)); err != nil {
					t.Fatalf("ReadFull: %v", err)
				}
				return r, func() {}, int64(len(content) - 10)
			},
			wantCL:   aws.Int64(int64(len(content) - 10)),
			wantBody: content[10:],
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
// end and restores the original position, that the uploaded content is read
// from the restored position rather than from the end of the reader, and that
// ContentLength reports the REMAINING bytes (end - current offset): the
// reader sits mid-stream at offset 4 of a 10-byte string.
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
	// starting at offset 4, not the empty remainder at the end. The
	// declared ContentLength must match those remaining 6 bytes (not the
	// stream's total length of 10): a larger declaration would break the
	// request on the wire (net/http rejects ContentLength larger than the
	// delivered body).
	if want := []byte("456789"); !bytes.Equal(call.body, want) {
		t.Errorf("uploaded body = %q, want %q", call.body, want)
	}
	if call.contentLength == nil || *call.contentLength != 6 {
		t.Errorf("ContentLength = %v, want 6", call.contentLength)
	}
}

// TestWriteContentLengthSeekFailure verifies that a seekable reader whose
// seeks fail falls back to nil ContentLength instead of erroring or reporting
// a wrong length.
func TestWriteContentLengthSeekFailure(t *testing.T) {
	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	spy := &seekSpy{r: strings.NewReader("0123456789"), failAt: 0, failOnceAt: -1}
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

// TestWriteContentLengthPartiallyConsumedFile pins branch precedence in
// content-length detection. A *os.File is both an fs.File and an io.Seeker,
// and the two answer differently once it has been partially consumed: Stat
// reports the total size, the seek offset reports the remaining bytes. The
// remaining length is the one that must win, because the declared
// ContentLength has to equal the delivered body length or net/http rejects
// the upload. Taking the fs.File branch first declares the total and delivers
// only the tail.
func TestWriteContentLengthPartiallyConsumedFile(t *testing.T) {
	const (
		bucket  = "test-bucket"
		key     = "partial-file-key"
		consume = 10
	)
	content := []byte("the quick brown fox jumps over the lazy dog")
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Partially consume the file: the upload must deliver only the tail.
	if _, err := io.ReadFull(f, make([]byte, consume)); err != nil {
		t.Fatal(err)
	}

	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	if _, err := write(context.Background(), up, bucket, key, f); err != nil {
		t.Fatalf("write returned error: %v", err)
	}
	call := rec.last()
	if call == nil {
		t.Fatal("no PutObject call recorded")
	}
	wantBody := content[consume:]
	if !bytes.Equal(call.body, wantBody) {
		t.Errorf("uploaded body = %q, want %q", call.body, wantBody)
	}
	if call.contentLength == nil {
		t.Fatal("ContentLength = nil, want non-nil")
	}
	if *call.contentLength != int64(len(wantBody)) {
		t.Errorf("ContentLength = %d, want remaining bytes %d", *call.contentLength, len(wantBody))
	}
	// net/http rejects requests whose declared ContentLength differs from
	// the delivered body length, so the two must always agree for the
	// request to be accepted.
	if *call.contentLength != int64(len(call.body)) {
		t.Errorf("ContentLength %d != delivered body length %d (request would be rejected)", *call.contentLength, len(call.body))
	}
}

// TestWriteContentLengthSharedReader verifies the sniff leaves a shared
// reader's position coherent: the caller consumes part of the reader, then
// hands the SAME reader to write(). The upload must deliver exactly the
// remaining bytes — the sniff must not leave the reader at EOF (the
// pre-fix silent-EOF failure) or misdeclare the length. The caller's
// accounting must add up (4 consumed + 6 uploaded = 10 total).
func TestWriteContentLengthSharedReader(t *testing.T) {
	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	r := strings.NewReader("0123456789")
	// The caller consumes the first 4 bytes, then shares the reader with
	// the upload path.
	if _, err := io.CopyN(io.Discard, r, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := write(context.Background(), up, "bucket", "shared-key", r); err != nil {
		t.Fatalf("write returned error: %v", err)
	}
	call := rec.last()
	if call == nil {
		t.Fatal("no PutObject call recorded")
	}
	if want := []byte("456789"); !bytes.Equal(call.body, want) {
		t.Errorf("uploaded body = %q, want %q (exactly the remaining bytes; the sniff must not skip or re-read any)", call.body, want)
	}
	if call.contentLength == nil || *call.contentLength != 6 {
		t.Errorf("ContentLength = %v, want 6 (remaining bytes)", call.contentLength)
	}
	// Full accounting: the caller read 4 bytes, the upload consumed the
	// remaining 6. Exactly 10 bytes total means the sniff neither rewound
	// nor skipped data, and the reader was not left dangling at EOF.
	if left := r.Len(); left != 0 {
		t.Errorf("reader has %d unread bytes after write, want 0 (exactly consumed to EOF)", left)
	}
}

// TestWriteContentLengthRestoreFailure pins the acceptance criterion that a
// failed restore seek is a hard error: the reader is left at an unknown
// position (typically EOF), so continuing would silently upload an empty
// body. write() must return the error and never issue PutObject.
func TestWriteContentLengthRestoreFailure(t *testing.T) {
	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	spy := newSeekSpy(strings.NewReader("0123456789"))
	// Position the reader mid-stream before the write.
	if _, err := spy.Seek(4, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	spy.seeks = nil
	// Seek order in sniffSeekerLength: 0=SeekCurrent, 1=SeekEnd,
	// 2=restore SeekStart. Failing index 2 simulates an unrestorable
	// reader (the pre-fix code silently uploaded from EOF here).
	spy.failAt = 2
	_, err := write(context.Background(), up, "bucket", "key", spy)
	if err == nil {
		t.Fatal("write() returned nil error when the restore seek failed")
	}
	// The error follows the fs contract: a *fs.PathError for the write.
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("errors.As(err, &*fs.PathError) = false, err = %T %v", err, err)
	}
	if pathErr.Op != "write" || pathErr.Path != "key" {
		t.Errorf("PathError = {%q %q}, want {%q %q}", pathErr.Op, pathErr.Path, "write", "key")
	}
	if !strings.Contains(pathErr.Err.Error(), "restore") {
		t.Errorf("PathError.Err = %v, want it to mention the failed restore", pathErr.Err)
	}
	// The critical fix: no PutObject may be issued after the sniff
	// failed, otherwise the upload would deliver an empty body.
	if rec.last() != nil {
		t.Error("PutObject was called even though the restore seek failed")
	}
}

// TestWriteContentLengthEndSeekFailure verifies the restore attempt after a
// failed seek-to-end probe: (a) if the original position can be restored,
// write() continues with nil ContentLength and uploads the REMAINING bytes
// from the restored position; (b) if the restore also fails, write() errors
// instead of uploading from an unknown position.
func TestWriteContentLengthEndSeekFailure(t *testing.T) {
	t.Run("restore succeeds", func(t *testing.T) {
		rec := &recordingUploader{}
		up := newTestUploader(t, rec)
		spy := newSeekSpy(strings.NewReader("0123456789"))
		if _, err := spy.Seek(4, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		spy.seeks = nil
		// Fail the SeekEnd probe (index 1) exactly once; the restore
		// SeekStart (index 2) then succeeds.
		spy.failOnceAt = 1
		if _, err := write(context.Background(), up, "bucket", "key", spy); err != nil {
			t.Fatalf("write returned error: %v", err)
		}
		call := rec.last()
		if call == nil {
			t.Fatal("no PutObject call recorded")
		}
		if call.contentLength != nil {
			t.Errorf("ContentLength = %d, want nil (length unknown after failed end-seek)", *call.contentLength)
		}
		if want := []byte("456789"); !bytes.Equal(call.body, want) {
			t.Errorf("uploaded body = %q, want %q (upload continues from the restored position)", call.body, want)
		}
	})
	t.Run("restore fails", func(t *testing.T) {
		rec := &recordingUploader{}
		up := newTestUploader(t, rec)
		spy := newSeekSpy(strings.NewReader("0123456789"))
		if _, err := spy.Seek(4, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		spy.seeks = nil
		// Fail SeekEnd (index 1); the position restore (index 2) fails
		// too, so the reader's position is unrecoverable.
		spy.failAt = 1
		if _, err := write(context.Background(), up, "bucket", "key", spy); err == nil {
			t.Fatal("write() returned nil error when the end-seek and its restore both failed")
		}
		if rec.last() != nil {
			t.Error("PutObject was called even though the reader position was unrecoverable")
		}
	})
}

// TestWriteContentLengthSeekerPastEnd verifies that a seekable reader
// positioned beyond its end reports no length (nil ContentLength) and the
// upload body reads as empty — the sniff must not error or declare a
// negative/zero length that would break the request on the wire.
func TestWriteContentLengthSeekerPastEnd(t *testing.T) {
	rec := &recordingUploader{}
	up := newTestUploader(t, rec)
	spy := newSeekSpy(strings.NewReader("0123456789"))
	if _, err := spy.Seek(15, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := write(context.Background(), up, "bucket", "key", spy); err != nil {
		t.Fatalf("write returned error: %v", err)
	}
	call := rec.last()
	if call == nil {
		t.Fatal("no PutObject call recorded")
	}
	if call.contentLength != nil {
		t.Errorf("ContentLength = %d, want nil (reader positioned past end)", *call.contentLength)
	}
	if len(call.body) != 0 {
		t.Errorf("uploaded body = %q, want empty", call.body)
	}
}
