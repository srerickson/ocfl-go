package s3

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// sniffSeekerLength returns the number of bytes remaining from the
// reader's current position to its end (end offset minus current
// offset). It restores the original position before returning, so a
// successful sniff leaves the caller's reader unmoved.
//
// Determining the length of a generic io.Seeker requires seeking to the
// end, which temporarily moves the reader. The original position is
// always restored before a length is returned; if the restore seek
// fails the reader is left at an unknown position (typically EOF) and
// an upload would silently deliver an empty or truncated body, so this
// returns an error instead of a length and the caller must not upload.
// A probe that fails without moving the reader (or whose position can
// still be restored) returns (-1, nil) so the caller leaves
// ContentLength unset and the SDK streams the body without a declared
// length.
func sniffSeekerLength(seeker io.Seeker) (int64, error) {
	cur, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return -1, nil
	}
	end, err := seeker.Seek(0, io.SeekEnd)
	if err != nil {
		// The offset is unspecified after a failed seek; try to get
		// back to the original position before giving up on the length.
		if _, rerr := seeker.Seek(cur, io.SeekStart); rerr != nil {
			return -1, fmt.Errorf("restore reader position %d after failed end-seek: %w", cur, rerr)
		}
		return -1, nil
	}
	if _, err := seeker.Seek(cur, io.SeekStart); err != nil {
		return -1, fmt.Errorf("restore reader position %d after length sniff: %w", cur, err)
	}
	if end < cur {
		return -1, nil
	}
	return end - cur, nil
}

func write(ctx context.Context, uploader *manager.Uploader, buck string, key string, r io.Reader, opts ...func(*s3.PutObjectInput)) (int64, error) {
	if !fs.ValidPath(key) || key == "." {
		return 0, pathErr("write", key, fs.ErrInvalid)
	}
	countReader := &countReader{Reader: r}
	var putInput s3.PutObjectInput
	for _, o := range opts {
		if o != nil {
			o(&putInput)
		}
	}
	putInput.Bucket = &buck
	putInput.Key = &key
	putInput.Body = countReader
	if putInput.ContentLength == nil {
		// try to get content length from r
		size := int64(-1)
		switch val := r.(type) {
		case *io.LimitedReader:
			size = val.N
		case io.Seeker:
			// Generic seekable reader (e.g. *os.File, *strings.Reader,
			// *bytes.Reader, or an fs.File that is also an io.Seeker such as
			// the file handle returned by Open): determine the REMAINING
			// length (end - current offset). A partially-consumed reader
			// (e.g. a strings.Reader after a first write, a *bytes.Reader
			// after a read, or a file read up to some offset) must report
			// only the bytes left, not the total size. *bytes.Reader used to
			// be special-cased with val.Size(), which reports the total size
			// of the underlying slice even for a partially-consumed reader,
			// so it is handled here like any other seeker.
			//
			// Sniffing a generic seeker requires temporarily seeking to
			// the end, so the reader must not be shared with other
			// goroutines for the duration of the write. The original
			// position is always restored before the length is used; if
			// the restore fails the reader would be left at EOF and the
			// upload would silently deliver an empty body, so write()
			// returns an error instead. If a probe fails without moving
			// the reader, ContentLength stays nil and the body is
			// streamed without a declared length.
			if n, err := sniffSeekerLength(val); err != nil {
				return 0, pathErr("write", key, err)
			} else if n >= 0 {
				size = n
			}
		case fs.File:
			// Non-seekable fs.File: fall back to the file's total size.
			if info, err := val.Stat(); err == nil {
				size = info.Size()
			}
		}
		if size > -1 {
			putInput.ContentLength = &size
		}
	}
	if _, err := uploader.Upload(ctx, &putInput); err != nil {
		return 0, &fs.PathError{Op: "write", Path: key, Err: err}
	}
	return countReader.size, nil
}

// countReader is a reader that updates a size counter with each read.
type countReader struct {
	io.Reader
	size int64
}

func (r *countReader) Read(p []byte) (int, error) {
	s, err := r.Reader.Read(p)
	r.size += int64(s)
	return s, err
}
