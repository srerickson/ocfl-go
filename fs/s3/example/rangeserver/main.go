// rangeserver is a simple HTTP server that serves objects from an S3 bucket
// with full support for HTTP Range requests. This enables features like:
// - Video seeking in browsers
// - Resumable downloads
// - Partial content fetching
//
// Usage:
//
//	go run main.go <bucket> [address]
//
// Examples:
//
//	go run main.go my-bucket              # serves on :8080
//	go run main.go my-bucket :9000         # serves on :9000
//
// Then access files like: http://localhost:8080/path/to/file.mp4
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go/fs/s3"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: rangeserver <bucket> [address]")
	}
	bucket := args[0]
	addr := ":8080"
	if len(args) > 1 {
		addr = args[1]
	}

	ctx := context.Background()
	fsys, err := newBucketFS(ctx, bucket)
	if err != nil {
		return fmt.Errorf("initializing S3: %w", err)
	}

	handler := &s3Handler{fsys: fsys}
	log.Printf("serving bucket %q on %s", bucket, addr)
	return http.ListenAndServe(addr, handler)
}

func newBucketFS(ctx context.Context, bucket string) (*s3.BucketFS, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	// Support custom endpoint via S3_ENDPOINT env var (useful for MinIO, LocalStack, etc.)
	var clientOpts []func(*s3v2.Options)
	if endpoint := os.Getenv("S3_ENDPOINT"); endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3v2.Options) {
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true
		})
	}
	return s3.NewBucketFS(s3v2.NewFromConfig(cfg, clientOpts...), bucket), nil
}

type s3Handler struct {
	fsys *s3.BucketFS
}

func (h *s3Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the object key from the URL path (strip leading /)
	key := strings.TrimPrefix(r.URL.Path, "/")
	if key == "" {
		http.Error(w, "missing object key", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Open the file from S3
	f, err := h.fsys.OpenFile(ctx, key)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			log.Printf("error opening %s: %v", key, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()

	// Get file info for size and modtime
	info, err := f.Stat()
	if err != nil {
		log.Printf("error stat %s: %v", key, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	size := info.Size()
	seeker := f.(io.ReadSeeker)

	// Indicate we support range requests
	w.Header().Set("Accept-Ranges", "bytes")

	// Parse Range header if present
	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" {
		// No range requested - serve full content
		log.Printf("%s %s: 200 OK (full content, %d bytes)", r.Method, key, size)
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			io.Copy(w, seeker)
		}
		return
	}

	// Parse the range request
	start, end, err := parseRangeHeader(rangeHeader, size)
	if err != nil {
		log.Printf("%s %s: 416 Range Not Satisfiable (request: %s, size: %d)", r.Method, key, rangeHeader, size)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Seek to the start position
	_, err = seeker.Seek(start, io.SeekStart)
	if err != nil {
		log.Printf("error seeking %s: %v", key, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Set headers for partial content
	contentLength := end - start + 1
	contentRange := fmt.Sprintf("bytes %d-%d/%d", start, end, size)
	log.Printf("%s %s: 206 Partial Content (request: %s, response: %s)", r.Method, key, rangeHeader, contentRange)
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("Content-Range", contentRange)
	w.WriteHeader(http.StatusPartialContent)

	if r.Method != http.MethodHead {
		// Copy only the requested range
		io.CopyN(w, seeker, contentLength)
	}
}

// parseRangeHeader parses a Range header like "bytes=0-499" or "bytes=500-" or "bytes=-500"
// Returns the start and end byte positions (inclusive)
func parseRangeHeader(header string, size int64) (start, end int64, err error) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, errors.New("invalid range unit")
	}

	spec := strings.TrimPrefix(header, "bytes=")

	// Handle multiple ranges - we only support single range
	if strings.Contains(spec, ",") {
		return 0, 0, errors.New("multiple ranges not supported")
	}

	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid range format")
	}

	startStr, endStr := parts[0], parts[1]

	// Handle suffix range: bytes=-500 (last 500 bytes)
	if startStr == "" {
		suffix, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, errors.New("invalid suffix range")
		}
		start = max(size-suffix, 0)
		return start, size - 1, nil
	}

	// Parse start position
	start, err = strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, errors.New("invalid start position")
	}

	// Handle open-ended range: bytes=500-
	if endStr == "" {
		return start, size - 1, nil
	}

	// Parse end position
	end, err = strconv.ParseInt(endStr, 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid end position")
	}

	// Clamp end to file size
	if end >= size {
		end = size - 1
	}

	// Validate range
	if start > end || start >= size {
		return 0, 0, errors.New("range not satisfiable")
	}

	return start, end, nil
}
