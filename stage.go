package ocfl

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"testing/fstest"

	"github.com/srerickson/ocfl-go/digest"
)

// StageBytes builds a stage from a map of filenames to file contents
func StageBytes(content map[string][]byte, alg digest.Algorithm, fixity ...digest.Algorithm) (*Stage, error) {
	mapFS := fstest.MapFS{}
	for file, bytes := range content {
		mapFS[file] = &fstest.MapFile{Data: bytes}
	}
	ctx := context.Background()
	return StageDir(ctx, NewFS(mapFS), ".", alg, fixity...)
}

// StageDir builds a stage based on the contents of the directory dir in FS.
// Files in dir and its subdirectories are digested with the given digest
// algorithms and added to the stage. Hidden files are ignored. The alg argument
// must be sha512 or sha256.
func StageDir(ctx context.Context, fsys FS, dir string, alg digest.Algorithm, fixity ...digest.Algorithm) (*Stage, error) {
	if alg.ID() != digest.SHA512.ID() && alg.ID() != digest.SHA256.ID() {
		return nil, fmt.Errorf("at least one algorithm (sha512 or sha256) must be provided")
	}
	files, walkErrFn := WalkFiles(ctx, fsys, dir)
	digests := files.IgnoreHidden().Digest(ctx, alg, fixity...)
	stage, err := digests.Stage()
	if err != nil {
		return nil, err
	}
	if err := walkErrFn(); err != nil {
		return nil, err
	}
	return stage, nil
}

// Stage is used to create/update objects.
type Stage struct {
	// State is a DigestMap representing the new object version state.
	State DigestMap
	// DigestAlgorithm is the primary digest algorithm (sha512 or sha256) used by the stage
	// state.
	DigestAlgorithm digest.Algorithm
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
	if al := s.DigestAlgorithm; al == nil || (al.ID() != digest.SHA512.ID() && al.ID() != digest.SHA256.ID()) {
		return errors.New("stage's digest algorithm must be 'sha512' or 'sha256'")
	}
	var err error
	for _, over := range stages {
		if s.DigestAlgorithm.ID() != over.DigestAlgorithm.ID() {
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

// ContentSource is used to access content with a given digest when creating and
// upadting objects.
type ContentSource interface {
	// GetContent returns an FS and path to a file in FS for a file with the given digest.
	// If no content is associated with the digest, fsys is nil and path is an empty string.
	GetContent(digest string) (fsys FS, path string)
}

// FixitySource is used to access alternate digests for content with a given
// digest (sha512 or sha256) when creating or updating objects.
type FixitySource interface {
	// GetFixity returns a DigestSet with alternate digests for the content with
	// the digest derived using the stage's primary digest algorithm.
	GetFixity(digest string) digest.Set
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

func (ps fixitySources) GetFixity(dig string) digest.Set {
	set := digest.Set{}
	for _, fixer := range ps {
		for fixAlg, fixDigest := range fixer.GetFixity(dig) {
			set[fixAlg] = fixDigest
		}
	}
	return set
}

type dirManifest struct {
	fs       FS
	baseDir  string
	manifest map[string]dirManifestEntry
}

func (s *dirManifest) ContentFS() FS {
	return s.fs
}

func (s *dirManifest) GetContent(digest string) (FS, string) {
	if s.fs == nil || s.manifest == nil || len(s.manifest[digest].paths) == 0 {
		return nil, ""
	}
	return s.fs, path.Join(s.baseDir, s.manifest[digest].paths[0])
}

func (s *dirManifest) GetFixity(dig string) digest.Set {
	return s.manifest[dig].fixity
}

type dirManifestEntry struct {
	paths  []string   // content paths relative to manifest baseDir
	fixity digest.Set // additional digests associate with paths
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

func (entry *dirManifestEntry) addFixity(fixity digest.Set) {
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
