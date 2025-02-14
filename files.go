package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"path"
	"runtime"
	"strings"

	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/internal/pipeline"
)

// Files returns a [FileSeq] for iterating over the named files in fsys.
func Files(fsys FS, names ...string) FileSeq {
	return FilesSub(fsys, ".", names...)
}

// Files returns a [FileSeq] for iterating over the named files, relative to the
// directory dir in fsys.
func FilesSub(fsys FS, dir string, files ...string) FileSeq {
	return func(yield func(*FileRef) bool) {
		for _, n := range files {
			ref := &FileRef{
				FS:      fsys,
				BaseDir: path.Clean(dir),
				Path:    path.Clean(n),
			}
			if !yield(ref) {
				break
			}
		}
	}
}

// WalkFiles returns a [FileSeq] for iterating over the files in dir and its
// subdirectories. If an error occurs while reading [FS], iteration terminates.
// The terminating error is returned by errFn
func WalkFiles(ctx context.Context, fsys FS, dir string) (files FileSeq, errFn func() error) {
	if walkFS, ok := fsys.(FileWalker); ok {
		files, errFn = walkFS.WalkFiles(ctx, dir)
		return
	}
	files, errFn = walkFiles(ctx, fsys, dir)
	return
}

// FileWalker is an [FS] with an optimized implementation of WalkFiles
type FileWalker interface {
	FS
	// WalkFiles returns a function iterator that yields all files in
	// dir and its subdirectories
	WalkFiles(ctx context.Context, dir string) (FileSeq, func() error)
}

// FileRef includes everything needed to access a file, including a reference to
// the [FS] where the file is stored. It may include file metadata from calling
// StatFile().
type FileRef struct {
	FS      FS          // The FS where the file is stored.
	BaseDir string      // parent directory relative to an FS.
	Path    string      // file path relative to BaseDir
	Info    fs.FileInfo // file info from StatFile (may be nil)
}

// FullPath returns the file's path relative to an [FS]
func (f FileRef) FullPath() string {
	return path.Join(f.BaseDir, f.Path)
}

// FullPathDir returns the full path of the directory where the
// file is stored.
func (f FileRef) FullPathDir() string {
	return path.Dir(f.FullPath())
}

// Namastes parses the file's name as a [Namaste] declaration and returns the
// result. If the file is not a namaste declaration, the zero-value is returned.
func (f FileRef) Namaste() Namaste {
	nam, _ := ParseNamaste(path.Base(f.Path))
	return nam
}

// Open return an [fs.File] for reading the contents of the file at f.
func (f *FileRef) Open(ctx context.Context) (fs.File, error) {
	return f.FS.OpenFile(ctx, f.FullPath())
}

// OpenObject opens f's directory as an *Object. If the the directory is not an
// existing object, an error is returned.
func (f *FileRef) OpenObject(ctx context.Context, opts ...ObjectOption) (*Object, error) {
	opts = append(opts, ObjectMustExist())
	return NewObject(ctx, f.FS, f.FullPathDir(), opts...)
}

// Stat() calls StatFile on the file at f and updates f.Info.
func (f *FileRef) Stat(ctx context.Context) error {
	stat, err := StatFile(ctx, f.FS, f.FullPath())
	f.Info = stat
	return err
}

// FileSeq is an iterator that yields *[FileRef] values.
type FileSeq iter.Seq[*FileRef]

// Filter returns a new FileSeq that yields values in files that satisfy the
// filter condition.
func (files FileSeq) Filter(filter func(*FileRef) bool) FileSeq {
	return func(yield func(*FileRef) bool) {
		for ref := range files {
			if !filter(ref) {
				continue
			}
			if !yield(ref) {
				break
			}
		}
	}
}

// IgnoreHidden returns a new FileSeq that does not included hidden files (files
// with a path element that begins with '.'). It only considers a FileRef's Path
// value, not its BaseDir.
func (files FileSeq) IgnoreHidden() FileSeq {
	return files.Filter(func(info *FileRef) bool {
		for _, part := range strings.Split(info.Path, "/") {
			if len(part) > 0 && part[0] == '.' {
				return false
			}
		}
		return true
	})
}

// Digest concurrently computes digests for each file in files. It is the same
// as DigestBatch with numgos set to [runtime.NumCPU](). The resulting iterator
// yields digest results or an error if the file could not be digestsed.
func (files FileSeq) Digest(ctx context.Context, alg digest.Algorithm, fixityAlgs ...digest.Algorithm) FileDigestsErrSeq {
	return files.DigestBatch(ctx, runtime.NumCPU(), alg, fixityAlgs...)
}

// DigestBatch concurrently computes digests for each file in files. The
// resulting iterator yields digest results or an error if the file could not be
// digestsed. If numgos is < 1, the value from [runtime.GOMAXPROCS](0) is used.
func (files FileSeq) DigestBatch(ctx context.Context, numgos int, alg digest.Algorithm, fixityAlgs ...digest.Algorithm) FileDigestsErrSeq {
	algs := make([]digest.Algorithm, 1+len(fixityAlgs))
	algs[0] = alg
	for i := 0; i < len(fixityAlgs); i++ {
		algs[i+1] = fixityAlgs[i]
	}
	digestFn := func(ref *FileRef) (*FileDigests, error) {
		f, err := ref.Open(ctx)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		digester := digest.NewMultiDigester(algs...)
		if _, err = io.Copy(digester, f); err != nil {
			return nil, fmt.Errorf("digesting %s: %w", ref.FullPath(), err)
		}
		fd := &FileDigests{
			FileRef:   *ref,
			Algorithm: alg,
			Digests:   digester.Sums(),
		}
		return fd, nil
	}
	return func(yield func(*FileDigests, error) bool) {
		for result := range pipeline.Results(iter.Seq[*FileRef](files), digestFn, numgos) {
			if !yield(result.Out, result.Err) {
				break
			}
		}
	}
}

// OpenObjects returns an iterator with results from calling OpenObject() on
// each *FileRef in files. Unlike [FileSeq.OpenObjectsBatch], objects are opened
// sequentially, in the same goroutine as the caller.
func (files FileSeq) OpenObjects(ctx context.Context, opts ...ObjectOption) iter.Seq2[*Object, error] {
	return func(yield func(*Object, error) bool) {
		for file := range files {
			if !yield(file.OpenObject(ctx, opts...)) {
				break
			}
		}
	}
}

// OpenObjectsBatch returns an iterator with results from calling OpenObjects()
// on each *FileRef in files. Unlike [FileSeq.OpenObjects], objects are opened
// in separate goroutines and may not be yielded in the same order as the input.
func (files FileSeq) OpenObjectsBatch(ctx context.Context, numgos int, opts ...ObjectOption) iter.Seq2[*Object, error] {
	openObj := func(ref *FileRef) (*Object, error) { return ref.OpenObject(ctx, opts...) }
	filesSeq := iter.Seq[*FileRef](files)
	return func(yield func(*Object, error) bool) {
		for result := range pipeline.Results(filesSeq, openObj, numgos) {
			if !yield(result.Out, result.Err) {
				break
			}
		}
	}
}

// Stat returns an iterator that yields a pointer to a new FileRef and error
// with results from calling [StatFile] for values in files (values from files
// are not modified).
func (files FileSeq) Stat(ctx context.Context) FileErrSeq {
	newFiles := func(yield func(*FileRef, error) bool) {
		for file := range files {
			newFile := *file
			stat, err := StatFile(ctx, file.FS, file.FullPath())
			newFile.Info = stat
			if !yield(&newFile, err) {
				break
			}
		}
	}
	return newFiles
}

// FileErrSeq is an iterator that yields *[FileRef] values or errors occuring
// while accessing a file.
type FileErrSeq iter.Seq2[*FileRef, error]

// UntilErr returns a new iterator yielding *FileRef values from seq that
// terminates on the first non-nil error in seq. The terminating error is
// returned by errFn.
func (fileErrs FileErrSeq) UntilErr() (FileSeq, func() error) {
	files, errFn := seqUntilErr(iter.Seq2[*FileRef, error](fileErrs))
	return FileSeq(files), errFn
}

// IgnoreErr returns an iterator of *[FileRef]s in seq that are not associated
// with an error.
func (fileErrs FileErrSeq) IgnoreErr() FileSeq {
	files := seqIgnoreErr(iter.Seq2[*FileRef, error](fileErrs))
	return FileSeq(files)
}

// FileDigests is a [FileRef] plus digest values of the file contents.
type FileDigests struct {
	FileRef
	Algorithm digest.Algorithm // primary digest algorithm (sha512 or sha256)
	Digests   digest.Set       // computed digest of file contents. Must include entry for the primary algorithm
}

// Validate confirms the digest values in pd using alogirthm definitions from
// reg. If the digests values do not match, the resulting error is a
// *[digest.DigestError].
func (pd FileDigests) Validate(ctx context.Context, reg digest.AlgorithmRegistry) error {
	f, err := pd.Open(ctx)
	if err != nil {
		return err
	}
	if err := pd.Digests.Validate(f, reg); err != nil {
		f.Close()
		var digestErr *digest.DigestError
		if errors.As(err, &digestErr) {
			digestErr.Path = pd.FullPath()
		}
		return err
	}
	return f.Close()
}

// FileDigestSeq is a
type FileDigestsSeq iter.Seq[*FileDigests]

// ValidateBatch concurrently validates sequence of FileDigests using numgos go
// routines. It returns an iterator of non-nill error values for any files that
// fail validation. If validation fails because a files content has changed, the
// yielded error is a *[digest.DigestError].
func (files FileDigestsSeq) ValidateBatch(ctx context.Context, reg digest.AlgorithmRegistry, numgos int) iter.Seq[error] {
	doDigest := func(pd *FileDigests) (struct{}, error) {
		err := pd.Validate(ctx, reg)
		return struct{}{}, err
	}
	return func(yield func(error) bool) {
		for result := range pipeline.Results(iter.Seq[*FileDigests](files), doDigest, numgos) {
			if result.Err != nil {
				if !yield(result.Err) {
					break
				}
			}
		}
	}
}

// FileDigestsErrSeq is an iterator that yields results and errors from a digesting
// file contents.
type FileDigestsErrSeq iter.Seq2[*FileDigests, error]

// Stage returns a new stage from the *[FileDigest]s in digests. An error is
// returned if digests yields an error or if the yielded *FileDigests are
// inconsisent (see FileDigestsSeq.Stage).
func (digests FileDigestsErrSeq) Stage() (*Stage, error) {
	files, errFn := digests.UntilErr()
	stage, err := files.Stage()
	if err != nil {
		return nil, err
	}
	if err := errFn(); err != nil {
		return nil, err
	}
	return stage, nil
}

// Stage builds a stage from values in digests. The stage's state uses the Path
// from each value in digests. The values in digests must have the same FS, primary
// digest algorithm, and base path, otherwise an error is returned.
func (digests FileDigestsSeq) Stage() (*Stage, error) {
	manifest := map[string]dirManifestEntry{}
	var primaryAlg digest.Algorithm
	var baseDir string
	var fsys FS
	for fileDigest := range digests {
		if fsys == nil {
			fsys = fileDigest.FS
		}
		if fsys != fileDigest.FS {
			return nil, errors.New("inconsistent backend FS for staged files")
		}
		if primaryAlg == nil {
			primaryAlg = fileDigest.Algorithm
		}
		if primaryAlg.ID() != fileDigest.Algorithm.ID() {
			return nil, errors.New("inconsistent digest algorithms for staged files")
		}
		if baseDir == "" {
			baseDir = fileDigest.BaseDir
		}
		if baseDir != fileDigest.BaseDir {
			return nil, errors.New("inconsistent base directory for staged files")
		}
		primary := fileDigest.Digests.Delete(primaryAlg.ID())
		if primary == "" {
			err := fmt.Errorf("missing %s value for %s", primaryAlg.ID(), fileDigest.FullPath())
			return nil, err
		}
		entry := manifest[primary]
		entry.addPaths(fileDigest.Path)
		entry.addFixity(fileDigest.Digests)
		manifest[primary] = entry
	}
	state := DigestMap{}
	for dig, entry := range manifest {
		state[dig] = entry.paths
	}
	dirMan := &dirManifest{
		fs:       fsys,
		baseDir:  baseDir,
		manifest: manifest,
	}
	return &Stage{
		State:           state,
		DigestAlgorithm: primaryAlg,
		ContentSource:   dirMan,
		FixitySource:    dirMan,
	}, nil
}

// UntilErr returns an iterator of *FileDigests from dfs that terminates on the
// first non-nil error in dfs. The terminating error is returned by errFn
func (dfs FileDigestsErrSeq) UntilErr() (FileDigestsSeq, func() error) {
	seq, errFn := seqUntilErr(iter.Seq2[*FileDigests, error](dfs))
	return FileDigestsSeq(seq), errFn
}

func walkFiles(ctx context.Context, fsys FS, dir string) (FileSeq, func() error) {
	var walkErr error
	seq := func(yield func(*FileRef) bool) {
		if !fs.ValidPath(dir) {
			walkErr = &fs.PathError{
				Err:  fs.ErrInvalid,
				Path: dir,
				Op:   "readir",
			}
			return
		}
		walkErr = fileWalk(ctx, fsys, dir, ".", yield)
	}
	errFn := func() error {
		if !errors.Is(walkErr, errBreakFileWalk) {
			return walkErr
		}
		return nil
	}
	return seq, errFn
}

// fileWalk calls yield for all files in dir and its subdirectories.
func fileWalk(ctx context.Context, fsys FS, walkRoot string, subDir string, yield func(*FileRef) bool) error {
	entries, err := fsys.ReadDir(ctx, path.Join(walkRoot, subDir))
	if err != nil {
		return err
	}
	for _, e := range entries {
		entryPath := path.Join(subDir, e.Name())
		ref := &FileRef{
			FS:      fsys,
			BaseDir: walkRoot,
			Path:    entryPath,
		}
		switch {
		case e.IsDir():
			if err := fileWalk(ctx, fsys, walkRoot, entryPath, yield); err != nil {
				return err
			}
		case !validFileType(e.Type()):
			return &fs.PathError{
				Path: entryPath,
				Err:  ErrFileType,
				Op:   `readdir`,
			}
		default:
			ref.Info, err = e.Info()
			if err != nil {
				return err
			}
			if !yield(ref) {
				return errBreakFileWalk
			}
		}
	}
	return nil
}

// special error returned from fileWalk when the
// range function is broken.
var errBreakFileWalk = errors.New("break")

// validFileType returns true if mode is ok for a file
// in an OCFL object.
func validFileType(mode fs.FileMode) bool {
	return mode.IsDir() || mode.IsRegular() || mode.Type() == fs.ModeIrregular
}

func seqUntilErr[T any](inSeq iter.Seq2[T, error]) (outSeq iter.Seq[T], errFn func() error) {
	var firstErr error
	outSeq = func(yield func(T) bool) {
		for v, err := range inSeq {
			if err != nil {
				firstErr = err
				break
			}
			if !yield(v) {
				break
			}
		}
	}
	errFn = func() error { return firstErr }
	return
}

func seqIgnoreErr[T any](inSeq iter.Seq2[T, error]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v, err := range inSeq {
			if err != nil {
				continue
			}
			if !yield(v) {
				break
			}
		}
	}
}
