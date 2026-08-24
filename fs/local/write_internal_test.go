package local

// Internal tests for write.go: the atomic-write sequence (temp file in the
// target directory, then rename) and the temp-file naming that supports it.
// These reach tempFileName, createTempFile, copyWithContext and tempPerm, so
// they cannot live in the external local_test package.
//
// atomicTempFiles here and tmpFilesIn in write_test.go do the same job on
// either side of that package boundary.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/carlmjohnson/be"
)

// atomicTempFiles returns the atomic-write temp files (".<base>.tmp-<random>")
// currently present in dir. After Write returns — successfully or not — this
// must be empty: any match is a leaked temp file.
func atomicTempFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	be.NilErr(t, err)
	return matches
}

// gateReader hands the writer one chunk, then blocks every subsequent Read
// until release is closed. first is closed as soon as the first chunk has
// been handed to the writer, which — because the temp file is created before
// the copy starts — guarantees the temp file exists once the test observes
// first. If err is set, it is returned as soon as the reader is released,
// simulating a source failure mid-write. It deliberately does not implement
// io.WriterTo, so both io.Copy (old impl) and copyWithContext (new impl)
// read it chunk by chunk.
type gateReader struct {
	data    []byte
	pos     int
	first   chan struct{} // closed after the first chunk is served
	release chan struct{} // subsequent reads block until this closes
	gated   bool          // whether the release gate has been consumed
	started bool          // whether first has been closed
	err     error         // optional error returned once released
}

func (r *gateReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	if r.pos > 0 && !r.gated {
		<-r.release
		r.gated = true
		if r.err != nil {
			return 0, r.err
		}
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if n > 0 && !r.started {
		r.started = true
		close(r.first)
	}
	return n, nil
}

// pacedReader yields its data in small chunks with a fixed pause between
// reads, so a Write fed from it takes tens of milliseconds. A write that
// copies in place holds a partial-content window open for that whole span,
// which is what gives concurrent readers a reliable chance to observe it.
type pacedReader struct {
	data  []byte
	pos   int
	step  int
	pause time.Duration
}

func (r *pacedReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	if r.pos > 0 {
		time.Sleep(r.pause)
	}
	end := r.pos + r.step
	if end > len(r.data) {
		end = len(r.data)
	}
	n := copy(p, r.data[r.pos:end])
	r.pos += n
	return n, nil
}

// patterned returns n bytes of a deterministic, non-repeating pattern so an
// exact content comparison cannot be fooled by a truncation that happens to
// land on a repeated character.
func patterned(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = seed + byte(i)*31
	}
	return b
}

func TestFS_WriteAtomic(t *testing.T) {
	t.Run("large payload is written completely and exactly", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		payload := patterned(8*1024*1024, 0xA0)
		n, err := fsys.Write(context.Background(), "big.bin", bytes.NewReader(payload))
		be.NilErr(t, err)
		be.Equal(t, int64(len(payload)), n)

		data, err := os.ReadFile(filepath.Join(tmpDir, "big.bin"))
		be.NilErr(t, err)
		be.True(t, bytes.Equal(payload, data))
		be.Equal(t, 0, len(atomicTempFiles(t, tmpDir)))
	})

	t.Run("cancellation mid-write leaves target absent and no temp files", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		payload := patterned(1024*1024, 0xB0)
		r := &gateReader{data: payload, first: make(chan struct{}), release: make(chan struct{})}

		var werr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, werr = fsys.Write(ctx, "test.bin", r)
		}()
		<-r.first
		cancel()
		close(r.release)
		wg.Wait()

		// The canceled write must report the cancellation, must not create a
		// file at the final path (new target), and must remove its temp file.
		// A write that checks ctx only up front would run to completion and
		// return nil here.
		be.True(t, errors.Is(werr, context.Canceled))
		if _, statErr := os.Stat(filepath.Join(tmpDir, "test.bin")); !os.IsNotExist(statErr) {
			t.Fatalf("file appeared at final path despite cancellation: %v", statErr)
		}
		be.Equal(t, 0, len(atomicTempFiles(t, tmpDir)))
	})

	t.Run("cancellation mid-write keeps previous content and no temp files", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		oldContent := patterned(256*1024, 0x4F)
		ctx := context.Background()
		_, err = fsys.Write(ctx, "test.bin", bytes.NewReader(oldContent))
		be.NilErr(t, err)

		ctx, cancel := context.WithCancel(ctx)
		payload := patterned(1024*1024, 0xB1)
		r := &gateReader{data: payload, first: make(chan struct{}), release: make(chan struct{})}

		var werr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, werr = fsys.Write(ctx, "test.bin", r)
		}()
		<-r.first
		cancel()
		close(r.release)
		wg.Wait()

		// Cancellation of an overwrite must leave the previous complete file
		// in place — the strictly harder case, since a write that truncates
		// the target up front has already destroyed it by this point.
		be.True(t, errors.Is(werr, context.Canceled))
		data, err := os.ReadFile(filepath.Join(tmpDir, "test.bin"))
		be.NilErr(t, err)
		be.True(t, bytes.Equal(oldContent, data))
		be.Equal(t, 0, len(atomicTempFiles(t, tmpDir)))
	})

	t.Run("failed overwrite preserves previous content", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		oldContent := patterned(256*1024, 0x4F)
		ctx := context.Background()
		_, err = fsys.Write(ctx, "test.bin", bytes.NewReader(oldContent))
		be.NilErr(t, err)

		errBoom := errors.New("source read boom")
		payload := patterned(1024*1024, 0xB2)
		r := &gateReader{data: payload, first: make(chan struct{}), release: make(chan struct{}), err: errBoom}

		var werr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, werr = fsys.Write(ctx, "test.bin", r)
		}()
		<-r.first
		close(r.release)
		wg.Wait()

		// A failing overwrite must preserve the previous content: the old
		// implementation truncates first and would leave a partial file.
		be.True(t, errors.Is(werr, errBoom))
		data, err := os.ReadFile(filepath.Join(tmpDir, "test.bin"))
		be.NilErr(t, err)
		be.True(t, bytes.Equal(oldContent, data))
		be.Equal(t, 0, len(atomicTempFiles(t, tmpDir)))
	})

	t.Run("concurrent readers never observe partial content", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)
		ctx := context.Background()
		target := "shared.bin"
		full := filepath.Join(tmpDir, target)

		oldContent := patterned(256*1024, 0x4F)
		newContent := patterned(1024*1024, 0xC0)
		_, err = fsys.Write(ctx, target, bytes.NewReader(oldContent))
		be.NilErr(t, err)

		// Readers begin sampling, and only then does the writer start, so the
		// sampling window provably overlaps the (deliberately slow) write.
		startWrite := make(chan struct{})
		writeDone := make(chan struct{})
		var readyWG sync.WaitGroup
		const readers = 3
		readyWG.Add(readers)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(writeDone)
			<-startWrite
			_, _ = fsys.Write(ctx, target, &pacedReader{data: newContent, step: 8 * 1024, pause: 250 * time.Microsecond})
		}()

		readErrs := make(chan string, readers)
		var rwg sync.WaitGroup
		rwg.Add(readers)
		for range readers {
			go func() {
				defer rwg.Done()
				readyWG.Done()
				for i := 0; i < 300; i++ {
					done := false
					select {
					case <-writeDone:
						done = true
					default:
					}
					f, err := os.Open(full)
					if err != nil {
						readErrs <- fmt.Sprintf("open failed: %v", err)
						return
					}
					data, err := io.ReadAll(f)
					f.Close()
					if err != nil {
						readErrs <- fmt.Sprintf("read failed: %v", err)
						return
					}
					if !bytes.Equal(data, oldContent) && !bytes.Equal(data, newContent) {
						readErrs <- fmt.Sprintf("observed partial content (%d bytes)", len(data))
						return
					}
					if done {
						return
					}
				}
			}()
		}
		readyWG.Wait()
		close(startWrite)
		rwg.Wait()
		close(readErrs)
		for msg := range readErrs {
			t.Errorf("%s", msg)
		}
		wg.Wait()

		final, err := os.ReadFile(full)
		be.NilErr(t, err)
		be.True(t, bytes.Equal(newContent, final))
		be.Equal(t, 0, len(atomicTempFiles(t, tmpDir)))
	})

	t.Run("temp file appears in target directory during write and is removed on success", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		payload := patterned(1024*1024, 0xD0)
		r := &gateReader{data: payload, first: make(chan struct{}), release: make(chan struct{})}

		var n int64
		var werr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, werr = fsys.Write(context.Background(), "test.bin", r)
		}()
		<-r.first

		// The write is blocked mid-copy with bytes already in the temp file.
		// The temp file must exist, live in the same directory as the target
		// (so the final rename is atomic on the same filesystem), and be the
		// only temp file. A write that copies straight to the final path
		// never creates one, and fails the first assertion.
		temps := atomicTempFiles(t, tmpDir)
		be.Equal(t, 1, len(temps))
		be.Equal(t, tmpDir, filepath.Dir(temps[0]))
		be.True(t, temps[0] != filepath.Join(tmpDir, "test.bin"))

		close(r.release)
		wg.Wait()
		be.NilErr(t, werr)
		be.Equal(t, int64(len(payload)), n)

		data, err := os.ReadFile(filepath.Join(tmpDir, "test.bin"))
		be.NilErr(t, err)
		be.True(t, bytes.Equal(payload, data))
		be.Equal(t, 0, len(atomicTempFiles(t, tmpDir)))
	})

	t.Run("temp file is removed when the write fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		errBoom := errors.New("source read boom")
		payload := patterned(1024*1024, 0xD1)
		r := &gateReader{data: payload, first: make(chan struct{}), release: make(chan struct{}), err: errBoom}

		var werr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, werr = fsys.Write(context.Background(), "test.bin", r)
		}()
		<-r.first

		temps := atomicTempFiles(t, tmpDir)
		be.Equal(t, 1, len(temps))
		be.Equal(t, tmpDir, filepath.Dir(temps[0]))

		close(r.release)
		wg.Wait()
		be.True(t, errors.Is(werr, errBoom))

		// Failed write on a new target: no file at the final path, and the
		// temp file that existed mid-write must be gone.
		if _, statErr := os.Stat(filepath.Join(tmpDir, "test.bin")); !os.IsNotExist(statErr) {
			t.Fatalf("file appeared at final path despite failure: %v", statErr)
		}
		be.Equal(t, 0, len(atomicTempFiles(t, tmpDir)))
	})
}

// TestFS_WriteOverwriteRegularFile covers overwriting an existing regular
// file through the full FS.Write path: the temp file is moved over the
// existing target, the content is replaced, and no temp files leak. On
// Windows this is the regression case for the rename helper — before it,
// the final os.Rename failed and the write returned an error.
func TestFS_WriteOverwriteRegularFile(t *testing.T) {
	root := t.TempDir()
	fsys, err := NewFS(root)
	be.NilErr(t, err)

	target := filepath.Join(root, "doc.txt")
	be.NilErr(t, os.WriteFile(target, []byte("old contents"), 0o644))

	n, err := fsys.Write(context.Background(), "doc.txt", strings.NewReader("new contents"))
	be.NilErr(t, err)
	be.Equal(t, int64(len("new contents")), n)

	got, err := os.ReadFile(target)
	be.NilErr(t, err)
	be.Equal(t, "new contents", string(got))
	be.Equal(t, 0, len(atomicTempFiles(t, root)))
}

// TestFS_WriteModePreservation pins that an overwrite keeps the existing
// target's permissions: Write must copy the old mode onto the temp file
// (chmod before the move) instead of leaving the default temp mode
// behind. The assertion is stability of the mode across the write, which
// holds on every platform: on POSIX the target is 0600 and the default
// temp mode would be 0644 &^ umask — an unpreserved write changes the
// reported mode and fails the test; on Windows 0600 is not representable
// (files are 0666 or 0444 read-only), so the equality assertion is the
// meaningful cross-platform statement, and exact mode bits are left to
// the POSIX-only symlink tests.
func TestFS_WriteModePreservation(t *testing.T) {
	root := t.TempDir()
	fsys, err := NewFS(root)
	be.NilErr(t, err)

	target := filepath.Join(root, "locked.txt")
	be.NilErr(t, os.WriteFile(target, []byte("old contents"), 0o600))
	before, err := os.Lstat(target)
	be.NilErr(t, err)

	if _, err := fsys.Write(context.Background(), "locked.txt", strings.NewReader("new contents")); err != nil {
		t.Fatalf("write over existing file: %v", err)
	}

	after, err := os.Lstat(target)
	be.NilErr(t, err)
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode after overwrite = %v, want preserved mode %v", after.Mode().Perm(), before.Mode().Perm())
	}
	got, err := os.ReadFile(target)
	be.NilErr(t, err)
	be.Equal(t, "new contents", string(got))
}

// dotLen and suffixLen are the fixed parts of a tempFileName result that
// wrap the (possibly truncated) base: the leading dot, the ".tmp-"
// separator, and the 16 hex random digits.
const (
	dotLen    = len(".")          // leading dot
	suffixLen = len(".tmp-") + 16 // ".tmp-" separator plus hex random
)

// basePortion extracts the truncated base embedded in a tempFileName result.
func basePortion(temp string) string {
	return temp[dotLen : len(temp)-suffixLen]
}

func TestTempFileNameUTF8(t *testing.T) {
	t.Run("short name is unchanged", func(t *testing.T) {
		temp := tempFileName("plain.bin")
		be.True(t, strings.HasPrefix(temp, ".plain.bin.tmp-"))
		be.Equal(t, dotLen+len("plain.bin")+suffixLen, len(temp))
		be.True(t, len(temp) <= 255)
		be.True(t, utf8.ValidString(temp))
	})

	t.Run("multibyte name under limit is not truncated", func(t *testing.T) {
		base := strings.Repeat("é", 100) // 200 bytes, under the 233-byte budget
		temp := tempFileName(base)
		be.True(t, strings.HasPrefix(temp, "."+base+".tmp-"))
		be.Equal(t, dotLen+len(base)+suffixLen, len(temp))
		be.True(t, utf8.ValidString(temp))
		createTempFileOnDisk(t, temp)
	})

	t.Run("long ASCII name truncates exactly at budget", func(t *testing.T) {
		base := strings.Repeat("a", 300)
		temp := tempFileName(base)
		trunc := basePortion(temp)
		be.Equal(t, 233, len(trunc))
		be.True(t, strings.HasPrefix(base, trunc))
		be.Equal(t, 255, len(temp))
		be.True(t, utf8.ValidString(temp))
		createTempFileOnDisk(t, temp)
	})

	t.Run("long multibyte name truncates on a rune boundary", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			rune  string
			count int
		}{
			{"latin-1 accent e", "é", 128}, // 256 bytes, 2 bytes/rune
			{"emoji", "😀", 64},             // 256 bytes, 4 bytes/rune
		} {
			t.Run(tc.name, func(t *testing.T) {
				base := strings.Repeat(tc.rune, tc.count)
				t.Logf("base byte length: %d (>255 required)", len(base))
				be.True(t, len(base) > 255)
				temp := tempFileName(base)

				// The full temp name must fit NAME_MAX and stay valid UTF-8:
				// no mid-rune truncation anywhere in the name.
				be.True(t, len(temp) <= 255)
				be.True(t, utf8.ValidString(temp))

				// Prefix correlation: the truncated base embedded in the temp
				// name is a prefix of the original base, and the truncation
				// only ever shortened it (never to zero for these sizes).
				trunc := basePortion(temp)
				be.True(t, strings.HasPrefix(base, trunc))
				be.True(t, len(trunc) > 0)
				be.True(t, len(trunc) <= 233)

				// The temp name must actually be creatable on the filesystem.
				createTempFileOnDisk(t, temp)
			})
		}
	})
}

// createTempFileOnDisk verifies that name can be created with O_EXCL in a
// fresh directory on the local filesystem, which is exactly what
// createTempFile does with tempFileName's result and what strict filesystems
// reject when a name is invalid UTF-8 or exceeds NAME_MAX.
func createTempFileOnDisk(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, tempPerm)
	be.NilErr(t, err)
	be.NilErr(t, f.Close())
}
