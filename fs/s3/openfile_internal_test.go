package s3

// Internal tests for openfile.go: cases that construct an s3File directly or
// reach its unexported fields, which the external s3_test package cannot do.

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	f := &s3File{
		ctx:    context.Background(),
		bucket: "test-bucket",
		key:    "test-key",
		body:   body,
		info:   &s3v2.HeadObjectOutput{ContentLength: aws.Int64(64)},
		logger: logger,
		offset: 5,
	}
	return f
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

// rangeStart parses an S3 Range header of the form "bytes=N-" (the only form
// s3File emits) and returns N; 0 when the header is absent or malformed.
func rangeStart(r *string) int64 {
	if r == nil || *r == "" {
		return 0
	}
	body := strings.TrimPrefix(*r, "bytes=")
	body, _, _ = strings.Cut(body, "-")
	n, err := strconv.ParseInt(body, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// testObjectData returns a deterministic byte slice whose bytes are their own
// index modulo 256, so any window of it is easy to assert against.
func testObjectData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}

// connCountingListener counts accepted TCP connections, which are exactly the
// HTTP/2 connections (each carries many multiplexed streams).
type connCountingListener struct {
	net.Listener
	conns atomic.Int64
}

func (l *connCountingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.conns.Add(1)
	}
	return c, err
}

// s3HTTPServer is a minimal S3 endpoint over h2: HEAD returns object
// metadata, GET serves range requests. It counts requests and HTTP/2 requests
// so tests can assert the exchange uses h2 exclusively.
type s3HTTPServer struct {
	data    []byte
	modTime time.Time
	reqs    atomic.Int64
	h2reqs  atomic.Int64
	badRng  atomic.Bool
}

func (s *s3HTTPServer) handler(w http.ResponseWriter, r *http.Request) {
	s.reqs.Add(1)
	if r.ProtoMajor == 2 {
		s.h2reqs.Add(1)
	}
	if key := strings.TrimPrefix(r.URL.Path, "/"); key != "test-bucket/obj" {
		http.Error(w, "unexpected key "+key, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodHead:
		w.Header().Set("Content-Length", strconv.Itoa(len(s.data)))
		w.Header().Set("ETag", `"s3test-etag"`)
		w.Header().Set("Last-Modified", s.modTime.Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		rng := r.Header.Get("Range")
		start := rangeStart(&rng)
		if rng != "" && !strings.HasPrefix(rng, "bytes=") {
			s.badRng.Store(true)
			http.Error(w, "malformed range header", http.StatusBadRequest)
			return
		}
		if start >= int64(len(s.data)) {
			http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(s.data)-int(start)))
		if start > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(s.data)-1, len(s.data)))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_, _ = w.Write(s.data[start:])
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

// newHTTP2S3Server starts an httptest TLS server with HTTP/2 enabled and an
// S3-shaped handler over data, returning the server handle (whose fields count
// requests), the connection-counting listener, and an *s3.Client configured to
// talk to the server.
func newHTTP2S3Server(t *testing.T, data []byte) (*s3HTTPServer, *connCountingListener, *s3v2.Client) {
	t.Helper()
	srv := &s3HTTPServer{data: data, modTime: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)}
	hs := httptest.NewUnstartedServer(http.HandlerFunc(srv.handler))
	hs.EnableHTTP2 = true
	ln := &connCountingListener{Listener: hs.Listener}
	hs.Listener = ln
	hs.StartTLS()
	t.Cleanup(hs.Close)

	pool := x509.NewCertPool()
	pool.AddCert(hs.Certificate())
	client := s3v2.NewFromConfig(aws.Config{
		Region:       "us-east-1",
		Credentials:  aws.AnonymousCredentials{},
		BaseEndpoint: aws.String(hs.URL),
		HTTPClient: &http.Client{Transport: &http.Transport{
			TLSClientConfig:    &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2:  true,
			DisableCompression: true,
		}},
	}, func(o *s3v2.Options) {
		o.UsePathStyle = true
	})
	return srv, ln, client
}

// TestS3File_SeekAfterPartialReadReusesHTTP2Connection opens an object
// through a real SDK client against an httptest HTTP/2 server, reads a small
// chunk, seeks (which closes the half-drained response body), and reads again
// — repeatedly. Closing a half-drained h2 body must not prevent connection
// reuse: every request in the sequence shares a single HTTP/2 connection,
// i.e. no connection churn per seek. Every request must also actually be h2,
// or the test would silently be exercising HTTP/1.1 semantics.
func TestS3File_SeekAfterPartialReadReusesHTTP2Connection(t *testing.T) {
	const size = 8 << 20
	data := testObjectData(size)
	srv, ln, s3cli := newHTTP2S3Server(t, data)

	f, err := openFile(context.Background(), s3cli, "test-bucket", "obj", nil)
	if err != nil {
		t.Fatalf("openFile: %v", err)
	}
	seeker, ok := f.(io.Seeker)
	if !ok {
		t.Fatal("opened file does not implement io.Seeker")
	}

	buf := make([]byte, 1024)
	// Partial read from the start (the SDK pairs one connection with the
	// preceding HEAD; count them all).
	n, err := f.Read(buf)
	if err != nil || n != len(buf) || !bytes.Equal(buf, data[:len(buf)]) {
		t.Fatalf("first Read = (%d, %v, %q), want (%d, nil, data[:1024])", n, err, buf, len(buf))
	}
	// Seek mid-object: this closes the half-drained response body.
	const mid = int64(2 << 20)
	if pos, err := seeker.Seek(mid, io.SeekStart); err != nil || pos != mid {
		t.Fatalf("Seek = (%d, %v), want (%d, nil)", pos, err, mid)
	}
	n, err = f.Read(buf)
	if err != nil || n != len(buf) || !bytes.Equal(buf, data[mid:mid+int64(len(buf))]) {
		t.Fatalf("Read after Seek = (%d, %v, %q), want (%d, nil, data[mid:mid+1024])", n, err, buf, len(buf))
	}
	// A few more seek+read cycles would each open a new connection if a
	// partial body close prevented reuse.
	for i := 0; i < 3; i++ {
		off := mid + int64(i+1)<<20
		if pos, err := seeker.Seek(off, io.SeekStart); err != nil || pos != off {
			t.Fatalf("Seek(%d) = (%d, %v), want (%d, nil)", off, pos, err, off)
		}
		n, err = f.Read(buf)
		if err != nil || n != len(buf) || !bytes.Equal(buf, data[off:off+int64(len(buf))]) {
			t.Fatalf("Read at %d = (%d, %v, %q), want (%d, nil, data[off:off+1024])", off, n, err, buf, len(buf))
		}
	}

	const wantReqs = 6 // 1 HEAD + 5 GETs
	if got := srv.reqs.Load(); got != wantReqs {
		t.Fatalf("requests = %d, want %d", got, wantReqs)
	}
	if got := srv.h2reqs.Load(); got != wantReqs {
		t.Fatalf("HTTP/2 requests = %d, want %d: the test must actually exercise h2, not HTTP/1.1", got, wantReqs)
	}
	if srv.badRng.Load() {
		t.Fatal("server saw a malformed Range header")
	}
	if got := ln.conns.Load(); got != 1 {
		t.Fatalf("HTTP/2 connections = %d, want 1: Seek after a partial Read must not prevent connection reuse (no connection churn)", got)
	}
}
