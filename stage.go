package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
	"github.com/srerickson/ocfl/internal/pathtree"
)

var errNoFS = errors.New("cannot add files to a stage without a backing FS")
var ErrExists = errors.New("the path exists and cannot be replaced")
var ErrCopyPath = errors.New("source or destination path is parent of the other")

// Stage is used to assemble the content (or "state") of an OCFL object prior to
// committing. A Stage can be backed by an FS, allowing new content to be added
// to the stage. Exising content can be removed, renamed, and copied, using
// Stage's methods. Use NewStage() create a new Stage.
type Stage struct {
	idx       *Index
	alg       digest.Alg
	srcFiles  map[string]struct{} // added source files
	writeLock sync.RWMutex
	fs        FS
	root      string // a prefix for all source paths in the index
	init      bool
}

// NewStage creates a new Stage. Stage options should include a source
// directory for adding files (StageRoot) or an initial index for setting existing
// content (StageIndex).
func NewStage(alg digest.Alg, opts ...StageOption) *Stage {
	stg := &Stage{
		idx: newEmptyIndex(),
		alg: alg,
	}
	for _, opt := range opts {
		opt(stg)
	}
	if stg.fs != nil && stg.root == "" {
		stg.root = "."
	}
	stg.init = true
	return stg
}

type StageOption func(*Stage)

// StageIndex is used in NewStage to set the stage's initial index. It is used,
// for example, to set the stage to match an existing object version.
func StageIndex(idx *Index) StageOption {
	return func(stage *Stage) {
		if !stage.init {
			// copy idx so it's not modified
			cp := idx.node.Copy()
			// clear all source paths in the index (they refer to existing files
			// in the object).
			pathtree.Walk(cp, func(_ string, n *pathtree.Node[IndexItem]) error {
				if n.IsDir() {
					return nil
				}
				n.Val.SrcPaths = nil
				return nil
			})
			stage.idx = &Index{node: *cp}
		}
	}
}

// StageRoot is used to set the backing FS and root directory for the stage.
// Setting the root is necessary for adding new files to the Stage. Setting the
// stage root does not automatically add the contents of the stage root to the
// stage. (Use AddExisting for that). After setting the root, other processes
// should not write to files in the root or stage corruption may occur.
func StageRoot(fsys FS, root string) StageOption {
	return func(stage *Stage) {
		if !stage.init {
			stage.fs = fsys
			stage.root = root
		}
	}
}

// Root returns stage's backing root directory. If none is set, it returns nil
// and an empty string.
func (stage *Stage) Root() (FS, string) {
	return stage.fs, stage.root
}

// DigestAlg returns stage's digest algorithm.
func (stage *Stage) DigestAlg() digest.Alg {
	return stage.alg
}

// AddAllFromRoot adds all files in the stage's root to the index. Previously staged
// files that aren't present in the stage root are removed. Checksums are
// calculated for all fies in the directory using the stage's digest algorith
// and optional fixity algorithms. When addings files to the stage, the file's path
// relative to the stage root is treated as a logical path.
func (stage *Stage) AddAllFromRoot(ctx context.Context, fixity ...digest.Alg) error {
	if stage.fs == nil {
		return errNoFS
	}
	newIndex := newEmptyIndex()
	// setup callback walks srcDir and adds files for checksum
	setup := func(addfn func(name string, algs ...digest.Alg) error) error {
		eachFileFn := func(name string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return addfn(name)
		}
		//
		return EachFile(ctx, stage.fs, stage.root, eachFileFn)
	}
	// result callback adds result to the new index
	results := func(name string, result digest.Set, err error) error {
		if err != nil {
			return err
		}
		// name is a path relative to stage.FS: trim the root prefix
		src := strings.TrimPrefix(name, stage.root+"/")
		info := IndexItem{
			SrcPaths: []string{src},
			Digests:  result,
		}
		return newIndex.node.SetFile(src, info)
	}
	// checksum file opener uses the stage FS
	open := func(name string) (io.Reader, error) {
		return stage.fs.OpenFile(ctx, name)
	}
	// append required options
	opts := []checksum.Option{
		checksum.WithOpenFunc(open),
		checksum.WithAlgs(fixity...),
		checksum.WithAlgs(stage.alg),
	}
	if err := checksum.Run(ctx, setup, results, opts...); err != nil {
		return fmt.Errorf("while staging files in '%s': %w ", stage.root, err)
	}
	// reset the stage
	stage.srcFiles = map[string]struct{}{}
	stage.idx = newIndex
	newIndex.Walk(func(name string, n *Index) error {
		if !n.IsDir() {
			stage.srcFiles[name] = struct{}{}
		}
		return nil
	})
	return nil
}

// Remove removes path n from the stage root.
func (stage *Stage) Remove(n string) error {
	if _, err := stage.idx.node.Remove(n); err != nil {
		return err
	}
	stage.idx.node.RemoveEmptyDirs()
	return nil
}

// Rename renames path src to dst in the stage root. If dst exists, it is
// over-written.
func (stage *Stage) Rename(src, dst string) error {
	return stage.idx.node.Rename(src, dst)
}

// Copy copies src to dst. An error is returned if dst exists, src is a parent
// of dst, or if dst is a parent of src.
func (stage *Stage) Copy(src, dst string) error {
	if _, _, err := stage.GetVal(dst); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, dst)
	}
	if src == "." || dst == "." || strings.HasPrefix(dst, src+"/") || strings.HasPrefix(src, dst+"/") {
		return fmt.Errorf("cannot copy '%s' to '%s': %w", src, dst, ErrCopyPath)
	}
	srcNode, err := stage.idx.node.Get(src)
	if err != nil {
		return err
	}
	cp := srcNode.Copy()
	return stage.idx.node.Set(dst, cp)
}

// GetVal returns the IndexItem stored in stage for path p along with a boolean
// indicating if path is a directory. An error is returned if no value is stored
// for p or if p is not a valid path. GetVal is safe to run from multiple go routines.
func (stage *Stage) GetVal(name string) (IndexItem, bool, error) {
	stage.writeLock.RLock()
	defer stage.writeLock.RUnlock()
	item, isdir, err := stage.idx.GetVal(name)
	return IndexItem(item), isdir, err
}

// Walk runs fn on all logical paths (both files and directories) in the stage,
// beginning with the root (".").
func (stage *Stage) Walk(fn IndexWalkFunc) error {
	return stage.idx.Walk(fn)
}

func (stage *Stage) Manifest(renameFunc func(string) string) (*digest.Map, error) {
	alg := stage.alg.ID()
	maker := &digest.MapMaker{}
	walkFn := func(p string, n *Index) error {
		if n.node.IsDir() {
			return nil
		}
		dig := n.node.Val.Digests[alg]
		if dig == "" {
			return fmt.Errorf("missing %s for '%s'", alg, p)
		}
		for _, src := range n.node.Val.SrcPaths {
			if renameFunc != nil {
				src = renameFunc(src)
			}
			if err := maker.Add(dig, src); err != nil {
				return err
			}
		}
		return nil
	}
	if err := stage.idx.Walk(walkFn); err != nil {
		return nil, err
	}
	return maker.Map(), nil
}

// AllManifests returns a map of digest.Maps with digest -> source path mappings for
// files in the stage. If the returned error is nil, the map will have at least
// one entry for the stage's digest algorithm. If any file is found in the stage
// without a value for the stage's digest algorithm, a non-nill error is
// returne.
func (stage *Stage) AllManifests(renameFunc func(string) string) (map[string]*digest.Map, error) {
	alg := stage.alg.ID()
	mapMakers := map[string]*digest.MapMaker{}
	walkFn := func(p string, n *Index) error {
		if n.node.IsDir() {
			return nil
		}
		digs := n.node.Val.Digests
		if digs[alg] == "" {
			return fmt.Errorf("missing %s for '%s'", alg, p)
		}
		for algID := range digs {
			if mapMakers[algID] == nil {
				mapMakers[algID] = &digest.MapMaker{}
			}
			for _, src := range n.node.Val.SrcPaths {
				if renameFunc != nil {
					src = renameFunc(src)
				}
				if err := mapMakers[algID].Add(digs[algID], src); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := stage.idx.Walk(walkFn); err != nil {
		return nil, err
	}
	maps := map[string]*digest.Map{}
	for alg, maker := range mapMakers {
		maps[alg] = maker.Map()
	}
	return maps, nil
}

// VersionState returns a digest map for the logical paths in the stage using
// the stage's primary digest algorithm.
func (stage *Stage) VersionState() *digest.Map {
	alg := stage.DigestAlg()
	maker := &digest.MapMaker{}
	walkFn := func(p string, n *Index) error {
		if n.node.IsDir() {
			return nil
		}
		dig, exists := n.node.Val.Digests[alg.ID()]
		if !exists {
			return fmt.Errorf("stage is missing required %s", alg.ID())
		}
		if err := maker.Add(dig, p); err != nil {
			return err
		}
		return nil
	}
	if err := stage.idx.Walk(walkFn); err != nil {
		// an error here represents a bug and
		// it should be addressed in testing.
		panic(err)
	}
	return maker.Map()
}

// UnsafeAdd is a low-level method for adding entries to the Stage. Because it
// can result in stage corruption, it should be avoided. Despite its name, UnsafeAdd
// is safe to run from multiple go routines.
func (stage *Stage) UnsafeAdd(lgcPath string, srcPath string, digests digest.Set) error {
	stage.writeLock.Lock()
	defer stage.writeLock.Unlock()
	if id := stage.alg.ID(); digests[id] == "" {
		return fmt.Errorf("missing required %s digest", id)
	}
	if stage.srcFiles == nil {
		stage.srcFiles = make(map[string]struct{})
	}
	info := IndexItem{Digests: digests, SrcPaths: []string{srcPath}}
	stage.srcFiles[srcPath] = struct{}{}
	if err := stage.idx.node.SetFile(lgcPath, info); err != nil {
		delete(stage.srcFiles, srcPath)
		return err
	}
	return nil
}

// WriteFile writes the contents of the io.Reader to a new file at the path, p,
// relative to the stage root. The stage's fs must be an ocfl.WriteFS and the
// source path must not have already been added to the stage. Checksums for the
// stage digest algorithm (and optional fixity algorithms) are created while the
// file is written. The path, p, is also treated as a logical path and added to
// the stage index, with a reference to the newly created file. WriteFile is
// safe to use from multiple go routines.
func (stage *Stage) WriteFile(ctx context.Context, p string, r io.Reader, fixity ...digest.Alg) (int64, error) {
	if stage.fs == nil {
		return 0, errNoFS
	}
	writeFS, ok := stage.fs.(WriteFS)
	if !ok {
		return 0, fmt.Errorf("stage's backing filesystem is not writable")
	}
	if stage.setSrcFileExists(p) {
		return 0, fmt.Errorf("cannot write to the previously staged source path: '%s': %w", p, ErrExists)
	}
	fullP := path.Join(stage.root, p)
	digester := digest.NewDigester(append(fixity, stage.alg)...)
	size, err := writeFS.Write(ctx, fullP, digester.Reader(r))
	if err != nil {
		stage.unsetSrcFile(p)
		return 0, err
	}
	if err := stage.UnsafeAdd(p, p, digester.Sums()); err != nil {
		return 0, err
	}
	return size, nil
}

// setSrcFileExists adds p to stage's srcFile map and
// returns a bool if it already existed
func (stage *Stage) setSrcFileExists(p string) bool {
	stage.writeLock.Lock()
	defer stage.writeLock.Unlock()
	if stage.srcFiles == nil {
		stage.srcFiles = make(map[string]struct{})
	}
	_, exists := stage.srcFiles[p]
	if !exists {
		stage.srcFiles[p] = struct{}{}
	}
	return exists
}

func (stage *Stage) unsetSrcFile(p string) {
	stage.writeLock.Lock()
	defer stage.writeLock.Unlock()
	delete(stage.srcFiles, p)
}
