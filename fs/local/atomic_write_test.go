package local

// atomic_write_test.go pins the atomic-write contract of FS.Write:
// write-to-temp-file-in-same-directory + rename. Each test is written to
// fail against the pre-atomic implementation (which opened the target with
// O_CREATE|O_TRUNC and copied in place with plain io.Copy):
//
//   - a large payload never appears truncated at the final path,
//   - cancellation or a source error mid-write leaves the final path
//     unchanged (absent for a new target, previous content for an existing
//     one) and removes the temp file,
//   - concurrent readers only ever observe the old or the new complete
//     content, never a partial file,
//   - the temp file lives in the target's own directory and is removed on
//     both success and failure.
//
// The file is self-contained (no helpers shared with localfs_test.go) and
// uses only the public FS API, so it compiles against the old implementation
// unchanged and fails there for behavioral reasons.
import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
// reads, so a Write fed from it takes tens of milliseconds. That keeps the
// old implementation's truncate-then-copy window open long enough for
// concurrent readers to reliably observe partial states.
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
		// The old implementation ignores the cancellation mid-copy, completes
		// the write and returns nil, so this fails there.
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
		// in place. The old implementation truncates the target up front, so
		// it fails here with either a partial file or a committed new write.
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
		// only temp file. The old implementation, which copies straight to the
		// final path, never creates one, so this fails there.
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
