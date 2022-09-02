package checksum

import (
	"hash"
	"io"
	"io/fs"
	"os"
	"sync"
)

type checksum struct {
	// FS to read filenames. If set, the Open function is ignored.
	FS fs.FS

	// Open is the function used to open file names added to the workq. Defaults to
	// os.Open.
	Open OpenFunc

	// Number of goroutines dedicated to processing checksums. Defaults to 1.
	NumGos int

	workQ   chan *job     // jobs todo
	resultQ chan *job     // job results
	cancel  chan struct{} // cancel remaining jobs
	errChan chan error    // return values for Close()
}

// HashSet is used to configure the hashes to calculate for a file
type HashSet map[string]func() hash.Hash

// HashResult is a map of hash results using same keys as the HashSet
type HashResult map[string][]byte

// OpenFunc is a function used to
type OpenFunc func(name string) (io.ReadCloser, error)

// CallbackFunc is a function used to handle results of a file digest.
type CallbackFunc func(name string, results HashResult, err error) error

// AddFunc is a funcion used to pass a filename and HashSet for checksuming
type AddFunc func(name string, algs HashSet) bool

// Run does concurrent checksumming.
func Run(setupFn func(AddFunc) error, cbFn CallbackFunc, opts ...optFunc) error {
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
	path string                      // path to file
	algs map[string]func() hash.Hash // hash name -> hash constructor
	sums map[string][]byte           // hash name -> result value
	err  error                       // any error from job
}

func (ch *checksum) open(cb CallbackFunc) error {
	gos := ch.NumGos
	if gos < 1 {
		gos = 1
	}
	if ch.FS != nil {
		ch.Open = func(name string) (io.ReadCloser, error) {
			return ch.FS.Open(name)
		}
	}
	if ch.Open == nil {
		ch.Open = defaultOpener
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
	var r io.ReadCloser
	r, j.err = ch.Open(j.path)
	if j.err != nil {
		return j
	}
	defer r.Close()
	var hashes = make(map[string]hash.Hash, len(j.algs))
	var writers []io.Writer
	for name, newHash := range j.algs {
		h := newHash()
		hashes[name] = h
		writers = append(writers, io.Writer(h))
	}
	multi := io.MultiWriter(writers...)
	if _, j.err = io.Copy(multi, r); j.err != nil {
		return j
	}
	j.sums = make(HashResult, len(j.algs))
	for name, h := range hashes {
		j.sums[name] = h.Sum(nil)
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
func (ch *checksum) add(path string, algs HashSet) bool {
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

func defaultOpener(name string) (io.ReadCloser, error) {
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

type optFunc func(*checksum)

// WithFS is a functional option used to set an FS backend for the checksum.
func WithFS(fsys fs.FS) optFunc {
	return func(c *checksum) {
		c.FS = fsys
	}
}

// WithOpenFunc is a functional options to set a function used to open filenames
// passed to the checksum process
func WithOpenFunc(open OpenFunc) optFunc {
	return func(c *checksum) {
		c.Open = open
	}
}

// WithNumGos sets the number of goroutines dedicated to processing checksums.
// Defaults to 1.
func WithNumGos(gos int) optFunc {
	return func(c *checksum) {
		c.NumGos = gos
	}
}
