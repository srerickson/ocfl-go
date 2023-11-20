package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
)

var (
	ErrStageNoFS  = errors.New("stage's FS is not set")
	ErrStageNoAlg = errors.New("stage's digest algorithm is not set")
	ErrExists     = errors.New("the path exists and cannot be replaced")
	ErrCopyPath   = errors.New("source or destination path is parent of the other")
)

// Stage is used to construct, add content to, and manipulate an object state prior
// to commit. At minimum a stage includes a digest algorithm (sha512 or sha256) and
// a logical, which may be exported with the State method. A stage also tracks an
// internal manifest of new content added to the stage.
type Stage struct {
	State DigestMap // StageState
	FS              // FS for adding new content to the stage
	Root  string    // base directory for all content
	Alg   string    // Primary digest algorith (sha512 or sha256)

	manifest stageManifest
}

// NewStage creates a new stage with the given digest algorithm, which should be
// either sha512 or sha256. If the stage will be used to update an object, the
// algorithm should match the object's. The new stage has an empty state and
// manifest and no backing FS. To add new content to the stage use AddFS or
// SetFS.
func NewStage(alg string) *Stage {
	return &Stage{
		Alg:      alg,
		manifest: map[string]stageEntry{},
		State:    DigestMap{},
	}
}

// SetFS sets the stage FS and root directory and clears any previously added
// content entries. It does not affect the stage's state.
func (stage *Stage) SetFS(fsys FS, dir string) {
	stage.FS = fsys
	stage.Root = path.Clean(dir)
	stage.manifest = stageManifest{}
}

// AddFS calls SetFS with the given FS and root directory and adds all files in
// the directory to the stage. Files in the root directory are digested using
// the stage's digest algorithm and optional fixity algorithms. Each file
// is added to the stage using UnsafeAddPath with file's path relative to the root
// directory and the calculated digest.
func (stage *Stage) AddFS(ctx context.Context, fsys FS, root string, fixity ...string) error {
	stage.SetFS(fsys, root)
	if err := stage.checkFSConfig(); err != nil {
		return err
	}
	algs := append([]string{stage.Alg}, fixity...)
	var walkErr error
	walkFS := func(addfn func(name string, algs ...string) bool) {
		eachFileFn := func(name string) error {
			addfn(name, algs...)
			return nil
		}
		walkErr = Files(ctx, stage.FS, Dir(stage.Root), eachFileFn)
	}
	// digest result: add results to the stage
	results := func(name string, result DigestSet, err error) error {
		if err != nil {
			return err
		}
		if stage.Root != "." {
			// Trim name so it's relative to root.
			name = strings.TrimPrefix(name, stage.Root+"/")
		}
		return stage.UnsafeAddPath(name, result)
	}
	// run checksum
	if err := DigestFS(ctx, stage, walkFS, results); err != nil {
		return fmt.Errorf("while staging root '%s': %w ", root, err)
	}
	return walkErr
}

// AddPath digests the file identified with name and adds the path to the stage.
// The path name and the associated digest are added to both the stage state and
// its manifest. The file is digested using the stage's primary digest algorith
// and any additional algorithms given by 'fixity'.
func (stage *Stage) AddPath(ctx context.Context, name string, fixity ...string) error {
	if err := stage.checkFSConfig(); err != nil {
		return err
	}
	fullName := path.Join(stage.Root, name)
	f, err := stage.OpenFile(ctx, fullName)
	if err != nil {
		return err
	}
	defer f.Close()
	digester := NewMultiDigester(append(fixity, stage.Alg)...)
	if _, err := io.Copy(digester, f); err != nil {
		return fmt.Errorf("during digest of '%s': %w", fullName, err)
	}
	return stage.UnsafeAddPath(name, digester.Sums())
}

// Manifest returns a DigestMap with content paths for digests in the stage
// state (if present in the stage manifest). Because manifest paths are not
// checked when they are added to the stage, it's possible for the manifest to
// be invalid, which is why this method can return an error.
func (stage Stage) Manifest() (DigestMap, error) {
	manifest := map[string][]string{}
	for _, digest := range stage.State.Digests() {
		if cont := stage.GetContent(digest); len(cont) > 0 {
			manifest[digest] = cont
		}
	}
	return NewDigestMap(manifest)
}

// GetContent returns the staged content paths for the given
// digest.
func (stage Stage) GetContent(dig string) []string {
	if stage.manifest == nil {
		return nil
	}
	return slices.Clone(stage.manifest[dig].paths)
}

// GetFixity returns altnerate digest values for the content with the primary
// digest value dig
func (stage Stage) GetFixity(dig string) DigestSet {
	fix := stage.manifest[dig].fixity
	set := make(DigestSet, len(fix))
	for k, v := range fix {
		set[k] = v
	}
	return set
}

// UnsafeAddPath adds name to the stage as both a logical path and a content
// path and associates name with the digests in the DigestSet. digests must
// include an entry with the stage's default digest algorithm. It is unsafe
// because neither the digest or the existence of the file are confirmed.
func (stage *Stage) UnsafeAddPath(name string, digests DigestSet) error {
	return stage.UnsafeAddPathAs(name, name, digests)
}

// UnsafeAddPathAs adds a logical path to the stage and the content path to the
// stage manifest. It is unsafe because neither the digest or the existence of
// the file are confirmed. If the content path is empty, the manifest is not
// updated; similarly, if the logical path is empty, the stage state is not
// updated. This allows for selectively adding entries to either the stage
// manifest or the stage state.
func (stage *Stage) UnsafeAddPathAs(content string, logical string, digests DigestSet) error {
	dig, fixity, err := splitDigests(digests, stage.Alg)
	if err != nil {
		return err
	}
	if logical != "" {
		newFile, err := NewDigestMap(map[string][]string{dig: {logical}})
		if err != nil {
			return err
		}
		merged, err := stage.State.Merge(newFile, true)
		if err != nil {
			return err
		}
		stage.State = merged
	}
	if content != "" {
		if stage.manifest == nil {
			stage.manifest = stageManifest{}
		}
		if err := stage.manifest.add(dig, content, fixity); err != nil {
			return err
		}
	}
	return nil
}

// UnsafeSetManifestFixty replaces the stage's existing content paths and fixity
// values to match manifest and fixity.
func (stage *Stage) UnsafeSetManifestFixty(manifest DigestMap, fixity map[string]DigestMap) error {
	newContents := stageManifest{}
	var err error
	manifest.EachPath(func(name, dig string) bool {
		altDigests := DigestSet{}
		for alg, dmap := range fixity {
			if fixDig := dmap.GetDigest(name); fixDig != "" {
				altDigests[alg] = fixDig
			}
		}
		err = newContents.add(dig, name, altDigests)
		return err == nil
	})
	if err != nil {
		return err
	}
	stage.manifest = newContents
	return nil
}

func (stage *Stage) checkFSConfig() error {
	if stage.FS == nil {
		return ErrStageNoFS
	}
	if !fs.ValidPath(stage.Root) {
		return fmt.Errorf("path '%s': %w", stage.Root, fs.ErrInvalid)
	}
	return nil
}

// stageManifest is a maps digest values to stageEntries.
type stageManifest map[string]stageEntry

func (man stageManifest) add(dig, name string, fixity DigestSet) error {
	// path exists under a different digest?
	for d, e := range man {
		if d == dig {
			continue
		}
		for _, p := range e.paths {
			if p == name {
				return fmt.Errorf("the path '%s' was previously assigned to a different digest value", name)
			}
		}
	}
	entry := man[dig]
	entry.addPath(name)
	entry.addFixity(fixity)
	man[dig] = entry
	return nil
}

type stageEntry struct {
	paths  []string  // content paths relative to Root in FS
	fixity DigestSet // additional digests associate with paths
}

func (entry *stageEntry) addPath(stagePath string) {
	i := sort.SearchStrings(entry.paths, stagePath)
	if i < len(entry.paths) && entry.paths[i] == stagePath {
		return // already present
	}
	entry.paths = append(entry.paths, "")
	copy(entry.paths[i+1:], entry.paths[i:])
	entry.paths[i] = stagePath
}

func (entry *stageEntry) addFixity(fixity DigestSet) {
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
