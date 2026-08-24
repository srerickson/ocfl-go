package s3

// Regression tests for s3File.Read/Seek concurrency semantics introduced by
// commit 49b0740 ("fs/s3: release mu during s3File.Read network I/O; Seek no
// longer stalls").
//
// Pinned behaviors:
//   - Seek returns promptly while a Read is blocked streaming a large object
//     (Read holds readMu, never mu, across GetObject and body.Read).
//   - A Read racing a Seek may return data from the pre-seek position; its
//     byte count never advances the offset (the generation check drops it).
//   - Concurrent Reads serialize on readMu: exactly one GetObject, one shared
//     body, and the offset tracks total consumption.
//   - Closing a half-drained body during Seek does not tear down the HTTP/2
//     connection: subsequent GetObject requests reuse it (no connection
//     churn), verified against a real SDK client and an httptest h2 server.
//
// All tests run under `go test -race ./fs/s3/` and must stay race-free.

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// seekPromptTimeout bounds how long a Seek may take while a Read is blocked
// mid-body. The refactored Seek only takes f.mu (a short snapshot lock), so it
// completes in microseconds; 2s gives an extremely slow CI the same pass/fail
// signal it would get on a fast machine.
const seekPromptTimeout = 2 * time.Second

// readResult carries one in-flight Read's outcome from a reader goroutine.
type readResult struct {
	n    int
	err  error
	data []byte
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

// gateAPI is a scripted OpenFileAPI for s3File concurrency tests. HEAD reports
// the object size; each GET returns a body that serves bytes from the range
// start. When gate is non-nil, every returned body's first Read blocks until
// gate is closed (letting the test hold a Read in flight) and signals entry
// into body.Read via started.
type gateAPI struct {
	data    []byte
	gate    chan struct{}
	started chan struct{}

	startOnce sync.Once
	getCalls  atomic.Int64
	mu        sync.Mutex
	ranges    []string
}

func (a *gateAPI) HeadObject(context.Context, *s3v2.HeadObjectInput, ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	return &s3v2.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(a.data))),
		ETag:          aws.String(`"gate-etag"`),
		LastModified:  aws.Time(time.Unix(1700000000, 0).UTC()),
	}, nil
}

func (a *gateAPI) GetObject(_ context.Context, in *s3v2.GetObjectInput, _ ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
	a.getCalls.Add(1)
	a.mu.Lock()
	a.ranges = append(a.ranges, aws.ToString(in.Range))
	a.mu.Unlock()
	start := rangeStart(in.Range)
	if start > int64(len(a.data)) {
		start = int64(len(a.data))
	}
	body := &gatedBody{data: a.data[start:], gate: a.gate}
	if a.gate != nil {
		body.started = a.started
		body.startOnce = &a.startOnce
	}
	return &s3v2.GetObjectOutput{Body: body}, nil
}

func (a *gateAPI) recordedRanges() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.ranges)
}

// gatedBody is an io.ReadCloser over a byte slice. When gate is non-nil, each
// Read first blocks until gate closes; the first Read signals started just
// before blocking. Close is a no-op: it models a response whose bytes are
// already buffered on the client, so a body closed by Seek still yields its
// buffered data to a concurrent in-flight body.Read.
type gatedBody struct {
	data      []byte
	off       int
	gate      chan struct{}
	started   chan struct{}
	startOnce *sync.Once
}

// copyBytes copies src into dst, returning the number of bytes copied. The
// builtin copy is shadowed at package scope by the S3 copy() upload helper,
// so tests cannot call it directly.
func copyBytes(dst, src []byte) int {
	n := len(dst)
	if len(src) < n {
		n = len(src)
	}
	for i := 0; i < n; i++ {
		dst[i] = src[i]
	}
	return n
}

func (b *gatedBody) Read(p []byte) (int, error) {
	if b.startOnce != nil {
		b.startOnce.Do(func() { close(b.started) })
	}
	if b.gate != nil {
		<-b.gate
	}
	if b.off >= len(b.data) {
		return 0, io.EOF
	}
	n := copyBytes(p, b.data[b.off:])
	b.off += n
	return n, nil
}

func (b *gatedBody) Close() error { return nil }

// newGateFile opens an s3File over a gateAPI serving data, returning the file
// and its API plus the started channel signaled when a Read is blocked inside
// body.Read on a gated body.
func newGateFile(t *testing.T, data []byte, gate chan struct{}) (*s3File, *gateAPI, chan struct{}) {
	t.Helper()
	started := make(chan struct{})
	api := &gateAPI{data: data, gate: gate, started: started}
	f, err := openFile(context.Background(), api, "test-bucket", "obj", nil)
	if err != nil {
		t.Fatalf("openFile: %v", err)
	}
	return f.(*s3File), api, started
}

// seekWhileReadBlocked waits until the reader goroutine is blocked inside
// body.Read (started), then calls f.Seek(to) from this goroutine and fails the
// test if the seek does not return within seekPromptTimeout — i.e. if Seek is
// ever blocked by an in-flight Read. release unblocks the reader on any
// failure path so the test can unwind without leaking blocked goroutines.
func seekWhileReadBlocked(t *testing.T, f *s3File, to int64, started chan struct{}, release func()) int64 {
	t.Helper()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		release()
		t.Fatal("reader goroutine never entered body.Read")
	}
	type seekResult struct {
		pos int64
		err error
	}
	done := make(chan seekResult, 1)
	go func() {
		pos, err := f.Seek(to, io.SeekStart)
		done <- seekResult{pos, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Seek returned error: %v", r.err)
		}
		if r.pos != to {
			t.Fatalf("Seek position = %d, want %d", r.pos, to)
		}
		return r.pos
	case <-time.After(seekPromptTimeout):
		release()
		t.Fatalf("Seek blocked for %v while a Read was in flight: Read must not hold a lock across network I/O", seekPromptTimeout)
		return 0
	}
}

// awaitReadResult waits for a reader goroutine's result, failing the test if
// it never arrives (the failure mode of a deadlock).
func awaitReadResult(t *testing.T, done chan readResult, what string) readResult {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not complete; possible deadlock", what)
		return readResult{}
	}
}

// testObjectData returns a deterministic byte slice. For i < 256 the bytes are
// their own index, which makes 32-byte windows at offsets 0, 32, ..., 224
// pairwise distinct (used to prove concurrent reads return non-overlapping
// data).
func testObjectData(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return data
}

// TestS3File_SeekPromptWhileReadBlocked starts a Read on a large object and,
// while that Read is blocked streaming data, calls Seek concurrently. Seek
// must return promptly even though the Read has not completed, and the whole
// exchange must not panic or deadlock. The stale in-flight Read returns
// pre-seek data whose byte count is discarded (the offset stays at the seek
// position), and the next Read serves the post-seek range.
func TestS3File_SeekPromptWhileReadBlocked(t *testing.T) {
	const size = 4 << 20
	data := testObjectData(size)
	gate := make(chan struct{})
	f, api, started := newGateFile(t, data, gate)

	const readLen = 4096
	buf := make([]byte, readLen)
	done := make(chan readResult, 1)
	go func() {
		n, err := f.Read(buf)
		done <- readResult{n: n, err: err, data: append([]byte(nil), buf[:n]...)}
	}()

	// Seek to the object's midpoint while the reader is blocked in body.Read.
	// This returns immediately on the refactored implementation; on the old
	// one (mu held across body.Read) it blocks until the gate opens and the
	// test fails at seekPromptTimeout.
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	const mid = size / 2
	seekWhileReadBlocked(t, f, mid, started, release)
	release()

	// The reader was blocked at position 0 when the seek landed, so it
	// deterministically returns pre-seek data; the generation check must
	// discard the byte count rather than move the new offset.
	rr := awaitReadResult(t, done, "blocked Read")
	if rr.err != nil {
		t.Fatalf("Read returned error: %v", rr.err)
	}
	if rr.n != readLen {
		t.Fatalf("Read n = %d, want %d", rr.n, readLen)
	}
	if !bytes.Equal(rr.data, data[:readLen]) {
		t.Fatalf("Read returned pre-seek data %q, want data[:%d]", rr.data, readLen)
	}
	if got := f.offset.Load(); got != mid {
		t.Fatalf("offset = %d, want %d (stale read must not advance the offset)", got, mid)
	}

	// A fresh Read observes the post-seek state: new range request, correct
	// bytes, offset advanced.
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("Read after Seek returned error: %v", err)
	}
	if n != readLen || !bytes.Equal(buf, data[mid:mid+readLen]) {
		t.Fatalf("Read after Seek = (%d, %q), want (%d, data[mid:mid+%d])", n, buf, readLen, readLen)
	}
	if got := f.offset.Load(); got != mid+readLen {
		t.Fatalf("offset after post-seek read = %d, want %d", got, mid+readLen)
	}

	// Exactly two fetches: the original full-object request and the post-seek
	// range request. The first carries no Range header (offset 0).
	if got := api.getCalls.Load(); got != 2 {
		t.Fatalf("GetObject calls = %d, want 2", got)
	}
	wantRanges := []string{"", fmt.Sprintf("bytes=%d-", mid)}
	if got := api.recordedRanges(); !slices.Equal(got, wantRanges) {
		t.Fatalf("GetObject ranges = %v, want %v", got, wantRanges)
	}
}

// TestS3File_ConcurrentReadsShareOneBody launches concurrent Readers on one
// file. The readMu must serialize them: exactly one GetObject, one shared
// body, non-overlapping data, and an offset that tracks total consumption.
func TestS3File_ConcurrentReadsShareOneBody(t *testing.T) {
	const size = 4 << 20
	data := testObjectData(size)
	f, api, _ := newGateFile(t, data, nil) // no gate: bodies deliver instantly

	const (
		nReaders = 8
		chunk    = 32
	)
	type result struct {
		off int
		buf []byte
		n   int
		err error
	}
	results := make([]result, nReaders)
	var wg sync.WaitGroup
	for i := 0; i < nReaders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			buf := make([]byte, chunk)
			n, err := f.Read(buf)
			// Reads are serialized by readMu and each advances the offset
			// before returning, so offset-before-this-read is current-n.
			results[i] = result{
				off: int(f.offset.Load()) - n,
				buf: append([]byte(nil), buf[:n]...),
				n:   n,
				err: err,
			}
		}(i)
	}
	wg.Wait()

	if got := api.getCalls.Load(); got != 1 {
		t.Fatalf("GetObject calls = %d, want 1 (concurrent Reads must share one body)", got)
	}
	seen := make([]bool, nReaders)
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("Read returned error: %v", r.err)
		}
		if r.n != chunk {
			t.Fatalf("Read n = %d, want %d", r.n, chunk)
		}
		slot := -1
		for k := 0; k < nReaders; k++ {
			if bytes.Equal(r.buf, data[k*chunk:k*chunk+chunk]) {
				slot = k
				break
			}
		}
		if slot == -1 {
			t.Fatalf("reader data %q matches no expected 32-byte window", r.buf)
		}
		if seen[slot] {
			t.Fatalf("window %d read by two goroutines: concurrent Reads interleaved on the body", slot)
		}
		seen[slot] = true
	}
	if got := f.offset.Load(); got != nReaders*chunk {
		t.Fatalf("offset = %d, want %d", got, nReaders*chunk)
	}
}

// TestS3File_ReadRacingSeekMayReturnPreSeekData pins the documented contract
// (see the s3File doc comment): a Read that races a Seek may return data from
// the pre-seek position, and its byte count is discarded — the next Read
// observes the post-seek state. The gated body makes the "may" deterministic:
// the Read is parked inside body.Read when the seek lands, and the bytes for
// that position are released only afterwards.
func TestS3File_ReadRacingSeekMayReturnPreSeekData(t *testing.T) {
	const size = 4 << 20
	data := testObjectData(size)
	gate := make(chan struct{})
	f, api, started := newGateFile(t, data, gate)

	const readLen = 4096
	buf := make([]byte, readLen)
	done := make(chan readResult, 1)
	go func() {
		n, err := f.Read(buf)
		done <- readResult{n: n, err: err, data: append([]byte(nil), buf[:n]...)}
	}()

	// Small seek while the Read is blocked at position 0.
	const seekTo = 100
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	seekWhileReadBlocked(t, f, seekTo, started, release)

	// Now release the pre-seek bytes: the racing Read returns them even
	// though the file position has moved.
	release()
	rr := awaitReadResult(t, done, "racing Read")
	if rr.err != nil {
		t.Fatalf("Read returned error: %v (the error arm is documented, but this deterministic setup must take the pre-seek-data arm)", rr.err)
	}
	if rr.n != readLen {
		t.Fatalf("Read n = %d, want %d", rr.n, readLen)
	}
	if !bytes.Equal(rr.data, data[:readLen]) {
		t.Fatalf("Read returned %q, want pre-seek data data[:%d]", rr.data, readLen)
	}
	// The stale byte count must not move the offset.
	if got := f.offset.Load(); got != seekTo {
		t.Fatalf("offset = %d, want %d (Read racing a Seek must not advance the offset with pre-seek data)", got, seekTo)
	}

	// The next Read observes the post-seek position.
	n, err := f.Read(buf)
	if err != nil || n != readLen {
		t.Fatalf("Read after seek = (%d, %v), want (%d, nil)", n, err, readLen)
	}
	if !bytes.Equal(buf, data[seekTo:seekTo+readLen]) {
		t.Fatalf("post-seek read returned %q, want data[%d:%d]", buf, seekTo, seekTo+readLen)
	}
	if got := f.offset.Load(); got != seekTo+readLen {
		t.Fatalf("offset = %d, want %d", got, seekTo+readLen)
	}
	if got := api.recordedRanges(); !slices.Equal(got, []string{"", fmt.Sprintf("bytes=%d-", seekTo)}) {
		t.Fatalf("GetObject ranges = %v, want [\"\" \"bytes=%d-\"]", got, seekTo)
	}
}

// TestS3File_ReadSeekConcurrentStress hammers the Read/Seek race under -race:
// a Read blocked in body.Read, one or two concurrent Seeks, an optional second
// Reader queued on readMu, then release — repeated with fresh files and random
// positions. It asserts the nondeterministic interleavings still preserve the
// invariants: no panic, no deadlock, stale byte counts dropped, correct
// post-seek data, and an offset consistent with the last seek.
func TestS3File_ReadSeekConcurrentStress(t *testing.T) {
	const size = 4 << 20
	data := testObjectData(size)
	rng := rand.New(rand.NewSource(1))

	const (
		readLen    = 4096
		extraLen   = 64
		iterations = 16
	)
	for iter := 0; iter < iterations; iter++ {
		gate := make(chan struct{})
		f, _, started := newGateFile(t, data, gate)
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { close(gate) }) }

		pos := rng.Int63n(size - readLen)
		buf := make([]byte, readLen)
		done := make(chan readResult, 1)
		go func() {
			n, err := f.Read(buf)
			done <- readResult{n: n, err: err, data: append([]byte(nil), buf[:n]...)}
		}()
		seekWhileReadBlocked(t, f, pos, started, release)

		if iter%2 == 1 {
			// A second seek while the Read is still in flight...
			pos2 := rng.Int63n(size - readLen)
			if pos2 == pos {
				pos2 = (pos2 + 1) % (size - readLen)
			}
			if _, err := f.Seek(pos2, io.SeekStart); err != nil {
				t.Fatalf("second Seek returned error: %v", err)
			}
			pos = pos2
			// ...and a second Reader that queues on readMu behind the
			// blocked one.
			extra := make([]byte, extraLen)
			extraDone := make(chan readResult, 1)
			go func() {
				n, err := f.Read(extra)
				extraDone <- readResult{n: n, err: err, data: append([]byte(nil), extra[:n]...)}
			}()
			release()
			rr1 := awaitReadResult(t, done, "stale Read")
			rr2 := awaitReadResult(t, extraDone, "queued Read")
			if rr1.err != nil || rr1.n != readLen || !bytes.Equal(rr1.data, data[:readLen]) {
				t.Fatalf("iter %d: stale Read = (%d, %v, %q), want (4096, nil, pre-seek data)", iter, rr1.n, rr1.err, rr1.data)
			}
			if rr2.err != nil || rr2.n != extraLen || !bytes.Equal(rr2.data, data[pos:pos+extraLen]) {
				t.Fatalf("iter %d: queued Read = (%d, %v, %q), want (%d, nil, data[%d:%d])", iter, rr2.n, rr2.err, rr2.data, extraLen, pos, pos+extraLen)
			}
			// Only the queued reader's bytes count: it ran after the
			// final seek and its generation matched.
			if got := f.offset.Load(); got != pos+extraLen {
				t.Fatalf("iter %d: offset = %d, want %d", iter, got, pos+extraLen)
			}
		} else {
			release()
			rr := awaitReadResult(t, done, "stale Read")
			if rr.err != nil || rr.n != readLen || !bytes.Equal(rr.data, data[:readLen]) {
				t.Fatalf("iter %d: stale Read = (%d, %v, %q), want (4096, nil, pre-seek data)", iter, rr.n, rr.err, rr.data)
			}
			if got := f.offset.Load(); got != pos {
				t.Fatalf("iter %d: offset = %d, want %d (stale read moved the offset)", iter, got, pos)
			}
		}

		// The file is still usable: a small fresh read serves the post-seek
		// range and advances the offset. In odd iterations the queued reader
		// already consumed extraLen bytes at the final position.
		checkStart := pos
		if iter%2 == 1 {
			checkStart = pos + extraLen
		}
		check := make([]byte, 128)
		n, err := f.Read(check)
		if err != nil || n != len(check) || !bytes.Equal(check, data[checkStart:checkStart+int64(len(check))]) {
			t.Fatalf("iter %d: post-seek check read = (%d, %v, %q), want (%d, nil, data[%d:%d])", iter, n, err, check, len(check), checkStart, checkStart+int64(len(check)))
		}
		if got := f.offset.Load(); got != checkStart+int64(len(check)) {
			t.Fatalf("iter %d: offset after check read = %d, want %d", iter, got, checkStart+int64(len(check)))
		}
	}
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
