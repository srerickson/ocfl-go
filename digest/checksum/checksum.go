package checksum

import (
	"encoding/hex"
	"hash"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/srerickson/ocfl/digest"
)

type checksum struct {
	fs       fs.FS        // fs to read filenames. If set, the Open function is ignored.
	openFunc OpenFunc     // Defaults to os.Open.
	numGos   int          // Number of goroutines dedicated to processing checksums. Defaults to 1.
	algs     []digest.Alg // default algs
	progress io.Writer
	workQ    chan *job     // jobs todo
	resultQ  chan *job     // job results
	cancel   chan struct{} // cancel remaining jobs
	errChan  chan error    // return values for Close()
}

// OpenFunc is a function used to
type OpenFunc func(name string) (io.Reader, error)

// CallbackFunc is a function used to handle results of a file digest.
type CallbackFunc func(name string, results digest.Set, err error) error

// AddFunc is a funcion used to pass a filename and algorithms to use for checksuming
type AddFunc func(name string, algs []digest.Alg) bool

// Run does concurrent checksumming.
func Run(setupFn func(AddFunc) error, cbFn CallbackFunc, opts ...Option) error {
	ch := checksum{}
	for _, o := range opts {
		o(&ch)
	}
	if err := ch.open(cbFn); err != nil {
		return err
	}
	runErr := setupFn(ch.add)
	cbErr := ch.close()
	if runErr != nil || cbErr != nil {
		return &Err{
			RunErr:      runErr,
			CallbackErr: cbErr,
		}
	}
	return nil
}

// checksum job
type job struct {
	path string       // path to file
	algs []digest.Alg // hash name -> hash constructor
	sums digest.Set   // hash name -> result value
	err  error        // any error from job
}

func (ch *checksum) open(cb CallbackFunc) error {
	gos := ch.numGos
	if gos < 1 {
		gos = 1
	}
	if ch.fs != nil {
		ch.openFunc = func(name string) (io.Reader, error) {
			return ch.fs.Open(name)
		}
	}
	if ch.openFunc == nil {
		ch.openFunc = defaultOpener
	}
	ch.workQ = make(chan *job, gos)
	ch.resultQ = make(chan *job, gos)
	ch.cancel = make(chan struct{})
	ch.errChan = make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < gos; i++ {
		wg.Add(1)
		go ch.worker(&wg)
	}
	go func() {
		defer close(ch.resultQ)
		wg.Wait()
	}()
	go func() {
		defer close(ch.errChan)
		defer close(ch.cancel)
		for j := range ch.resultQ {
			if err := cb(j.path, j.sums, j.err); err != nil {
				ch.errChan <- err
				return
			}
		}
	}()
	return nil
}

func (ch *checksum) worker(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ch.cancel:
			return
		case j, ok := <-ch.workQ:
			if !ok {
				return
			}
			ch.resultQ <- ch.doJob(j)
		}
	}
}

func (ch *checksum) doJob(j *job) *job {
	var r io.Reader
	r, j.err = ch.openFunc(j.path)
	if j.err != nil {
		return j
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	multiLen := len(j.algs)
	if ch.progress != nil {
		multiLen += 1
	}
	var hashes = make(map[digest.Alg]hash.Hash, multiLen)
	var writers = make([]io.Writer, 0, multiLen)
	for _, alg := range j.algs {
		h := alg.New()
		hashes[alg] = h
		writers = append(writers, io.Writer(h))
	}
	if ch.progress != nil {
		writers = append(writers, ch.progress)
	}
	_, j.err = io.Copy(io.MultiWriter(writers...), r)
	if j.err != nil {
		return j
	}
	j.sums = make(digest.Set, len(j.algs))
	for name, h := range hashes {
		j.sums[name] = hex.EncodeToString(h.Sum(nil))
	}
	return j
}

// close is used to signal that no additional paths will be added.
// close blocks until callbacks have been called for previously added paths
// (or returns an error)
func (ch *checksum) close() error {
	close(ch.workQ)     // no more work
	return <-ch.errChan // block until all callbacks called
}

// add adds a checksum job for path to the Pipe.
func (ch *checksum) add(path string, algs []digest.Alg) bool {
	if len(algs) == 0 {
		algs = ch.algs
	}
	j := &job{
		path: path,
		algs: algs,
	}
	select {
	case <-ch.cancel:
		return false
	case ch.workQ <- j:
		return true
	}
}

func defaultOpener(name string) (io.Reader, error) {
	return os.Open(name)
}

// Err is an error type for error returned by Run
type Err struct {
	RunErr      error // error returned by setup function during Run()
	CallbackErr error // error returned by callback function during Run()
}

// Error implements the error interface for PipeErr
func (err Err) Error() string {
	if err.CallbackErr != nil && err.RunErr != nil {
		return err.RunErr.Error() + "; " + err.CallbackErr.Error()
	}
	if err.CallbackErr != nil {
		return err.CallbackErr.Error()
	}
	if err.RunErr != nil {
		return err.RunErr.Error()
	}
	return ""
}

// Unwrap implements errors.Unwrap for PipeErr
func (err Err) Unwrap() error {
	if err.CallbackErr == nil {
		return err.RunErr
	}
	return err.CallbackErr
}

type Option func(*checksum)

// WithFS is a functional option used to set an FS backend for the checksum.
func WithFS(fsys fs.FS) Option {
	return func(c *checksum) {
		c.fs = fsys
	}
}

// WithOpenFunc is a functional options to set a function used to open filenames
// passed to the checksum process
func WithOpenFunc(open OpenFunc) Option {
	return func(c *checksum) {
		c.openFunc = open
	}
}

// WithNumGos sets the number of goroutines dedicated to processing checksums.
// Defaults to 1.
func WithNumGos(gos int) Option {
	return func(c *checksum) {
		c.numGos = gos
	}
}

func WithProgress(w io.Writer) Option {
	return func(c *checksum) {
		c.progress = w
	}
}

func WithDigest(d digest.Alg) Option {
	return func(c *checksum) {
		for _, a := range c.algs {
			if a == d {
				return
			}
		}
		c.algs = append(c.algs, d)
	}
}

var SHA256 = WithDigest(digest.SHA256)

// func SHA512() Option {
// 	return WithDigest(digest.SHA512)
// }

// func MD5() Option {
// 	return WithDigest(digest.MD5)
// }
// func SHA1() Option {
// 	return WithDigest(digest.SHA1)
// }

// func BLAKE2B() Option {
// 	return WithDigest(digest.BLAKE2B)
// }
