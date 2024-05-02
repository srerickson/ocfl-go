package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing/fstest"
)

// Stage is used to create/update objects.
type Stage struct {
	// State is a DigestMap representing the new object version state.
	State DigestMap
	// DigestAlgorithm is the primary digest algorithm (sha512 or sha256) used by the stage
	// state.
	DigestAlgorithm string
	// ContentSource is used to access new content needed to construct
	// an object. It may be nil
	ContentSource
	// FixitySource is used to access fixity information for new
	// content. It may be nil
	FixitySource
}

// HasContent returns true if the stage's content source provides an FS and path
// for the digest
func (s Stage) HasContent(digest string) bool {
	if s.ContentSource == nil {
		return false
	}
	f, p := s.ContentSource.GetContent(digest)
	return f != nil && p != ""
}

// Overlay merges the state and content/fixity sources from all stages into s.
// All stages mush share the same digest algorithm.
func (s *Stage) Overlay(stages ...*Stage) error {
	if s.State == nil {
		s.State = DigestMap{}
	}
	if alg := s.DigestAlgorithm; alg == "" || (alg != SHA512 && alg != SHA256) {
		return errors.New("stage's digest algorithm must be 'sha512' or 'sha256'")
	}
	var err error
	for _, over := range stages {
		if s.DigestAlgorithm != over.DigestAlgorithm {
			return errors.New("can't overlay stage with different digest algorithm than the base")
		}
		s.State, err = s.State.Merge(over.State, true)
		if err != nil {
			return err
		}
		s.addContentSource(over.ContentSource)
		s.addFixitySource(over.FixitySource)
	}
	if err := s.State.Valid(); err != nil {
		return err
	}
	return nil
}

func (s *Stage) addContentSource(cs ContentSource) {
	var sources contentSources
	switch current := s.ContentSource.(type) {
	case contentSources:
		sources = current
	case nil:
	default:
		sources = contentSources{current}
	}
	switch p := cs.(type) {
	case contentSources:
		sources = append(sources, p...)
	case nil:
	default:
		sources = append(sources, p)
	}
	s.ContentSource = sources
}

func (s *Stage) addFixitySource(fs FixitySource) {
	var sources fixitySources
	switch current := s.FixitySource.(type) {
	case fixitySources:
		sources = current
	case nil:
	default:
		sources = fixitySources{current}
	}
	switch p := fs.(type) {
	case fixitySources:
		sources = append(sources, p...)
	case nil:
	default:
		sources = append(sources, p)
	}

	s.FixitySource = sources
}

// ContentSource is used to access content with a given digest when
// creating and upadting objects.
type ContentSource interface {
	// GetContent returns an FS and path to a file in FS for a file with the given digest.
	// If no content is associated with the digest, fsys is nil and path is an empty string.
	GetContent(digest string) (fsys FS, path string)
}

// FixitySource is used to access alternate digests for content with a given digest
// (sha512 or sha256) when creating or updating objects.
type FixitySource interface {
	// GetFixity returns a DigestSet with alternate digests for the content with
	// the digest derrived using the stage's primary digest algorithm.
	GetFixity(digest string) DigestSet
}

type contentSources []ContentSource

func (ps contentSources) GetContent(digest string) (FS, string) {
	for _, provider := range ps {
		fsys, pth := provider.GetContent(digest)
		if fsys != nil {
			return fsys, pth
		}
	}
	return nil, ""
}

type fixitySources []FixitySource

func (ps fixitySources) GetFixity(digest string) DigestSet {
	set := DigestSet{}
	for _, fixer := range ps {
		for fixAlg, fixDigest := range fixer.GetFixity(digest) {
			set[fixAlg] = fixDigest
		}
	}
	return set
}

// StageDir builds a stage based on the contents of the directory dir in FS.
// All files in dir and its subdirectories are digested with the given digest
// algs and added to the stage. The algs must include sha512 or sha256 or an
// error is returned
func StageDir(ctx context.Context, fsys FS, dir string, algs ...string) (*Stage, error) {
	if len(algs) < 1 || (algs[0] != SHA512 && algs[0] != SHA256) {
		return nil, fmt.Errorf("must use sha512 or sha256 as the primary digest algorithm for the stage")
	}
	if !fs.ValidPath(dir) {
		return nil, fmt.Errorf("invalid stage directory: %q", dir)
	}
	alg := algs[0]
	dirMan := &dirManifest{
		fs:       fsys,
		root:     dir,
		manifest: map[string]dirManifestEntry{},
	}
	var walkErr error
	walkFS := func(digestFile func(name string, algs []string) bool) {
		// add files to digest work queue
		Files(ctx, dirMan.fs, dir)(func(info FileInfo, err error) bool {
			if err != nil {
				walkErr = err
				return false
			}
			digestFile(info.Path, algs)
			return true
		})
	}
	// digest result: add results to the stage
	var digestErr error
	Digest(ctx, dirMan.fs, walkFS)(func(r DigestResult, err error) bool {
		name := r.Path
		if err != nil {
			digestErr = err
			return false
		}
		if dirMan.root != "." {
			// Trim name so it's relative to root, not FS
			name = strings.TrimPrefix(name, dirMan.root+"/")
		}
		primary, fixity, err := splitDigests(r.Digests, alg)
		if err != nil {
			digestErr = err
			return false
		}
		if dirMan.manifest == nil {
			dirMan.manifest = make(map[string]dirManifestEntry)
		}
		entry := dirMan.manifest[primary]
		entry.addPaths(name)
		entry.addFixity(fixity)
		dirMan.manifest[primary] = entry
		return true
	})
	// run checksum
	if err := errors.Join(walkErr, digestErr); err != nil {
		return nil, err
	}
	state := DigestMap{}
	for dig, entry := range dirMan.manifest {
		state[dig] = entry.paths
	}
	return &Stage{
		State:           state,
		DigestAlgorithm: alg,
		ContentSource:   dirMan,
		FixitySource:    dirMan,
	}, nil
}

type dirManifest struct {
	fs       FS
	root     string
	manifest map[string]dirManifestEntry
}

func (s *dirManifest) ContentFS() FS {
	return s.fs
}

func (s *dirManifest) GetContent(digest string) (FS, string) {
	if s.fs == nil || s.manifest == nil || len(s.manifest[digest].paths) == 0 {
		return nil, ""
	}
	return s.fs, path.Join(s.root, s.manifest[digest].paths[0])
}

func (s *dirManifest) GetFixity(dig string) DigestSet {
	return s.manifest[dig].fixity
}

type dirManifestEntry struct {
	paths  []string  // content paths relative to Root in FS
	fixity DigestSet // additional digests associate with paths
}

func (entry *dirManifestEntry) addPaths(paths ...string) {
	for _, stagePath := range paths {
		i := sort.SearchStrings(entry.paths, stagePath)
		if i < len(entry.paths) && entry.paths[i] == stagePath {
			return // already present
		}
		entry.paths = append(entry.paths, "")
		copy(entry.paths[i+1:], entry.paths[i:])
		entry.paths[i] = stagePath
	}
}

func (entry *dirManifestEntry) addFixity(fixity DigestSet) {
	if len(fixity) == 0 {
		return
	}
	if entry.fixity == nil {
		entry.fixity = fixity
		return
	}
	for alg, dig := range fixity {
		entry.fixity[alg] = dig
	}
}

func splitDigests(set DigestSet, alg string) (string, DigestSet, error) {
	newSet := DigestSet{}
	dig := ""
	for setAlg, setVal := range set {
		if setAlg == alg {
			dig = setVal
			continue
		}
		newSet[setAlg] = setVal
	}
	if dig == "" {
		return "", nil, fmt.Errorf("missing %s value", alg)
	}
	return dig, newSet, nil
}

// StageBytes builds a stage from a map of filenames to file contents
func StageBytes(content map[string][]byte, algs ...string) (*Stage, error) {
	mapFS := fstest.MapFS{}
	for file, bytes := range content {
		mapFS[file] = &fstest.MapFile{Data: bytes}
	}
	ctx := context.Background()
	return StageDir(ctx, NewFS(mapFS), ".", algs...)
}
