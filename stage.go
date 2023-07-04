package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"runtime"
	"sort"
	"strings"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
	"github.com/srerickson/ocfl/internal/pathtree"
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
	FS              // FS for adding new content to the stage
	Root string     // base directory for all content
	Alg  digest.Alg // Primary digest algorith (sha512 or sha256)

	// Number of go routines to use for concurrent digest during AddFS
	Concurrency int

	manifest stageManifest
	state    *pathtree.Node[string] // mutable directory structure
}

// NewStage creates a new stage with the given digest algorithm and initial
// state. The digest algorithm should be either sha512 or sha256. If the stage
// will be used to update an object, the algorithm should match the object's.
// The resulting stage has no backing FS, so content cannot be added. To add new
// content to the stage use AddFS or SetFS.
func NewStage(alg digest.Alg, state digest.Map) (*Stage, error) {
	stage := &Stage{
		Alg:      alg,
		manifest: map[string]stageEntry{},
	}
	if err := stage.SetState(state); err != nil {
		return nil, err
	}
	return stage, nil
}

// SetFS sets the stage FS and root directory and clears any previously added
// content entries. It does not affect the stage's state.
func (stage *Stage) SetFS(fsys FS, dir string) {
	stage.FS = fsys
	stage.Root = path.Clean(dir)
	stage.manifest = stageManifest{}
}

// SetState sets the stage's state, replacing any previous values.
func (stage *Stage) SetState(state digest.Map) error {
	if err := state.Valid(); err != nil {
		return err
	}
	newState := pathtree.NewDir[string]()
	if err := state.EachPath(func(n, d string) error {
		return newState.SetFile(n, d)
	}); err != nil {
		return err
	}
	stage.state = newState
	return nil
}

// AddFS calls SetFS with the given FS and root directory and adds all files in
// the directory to the stage. Files in the root directory are digested using
// the stage's digest algorithm and optional fixity algorithms. Each file
// is added to the stage using UnsafeAddPath with file's path relative to the root
// directory and the calculated digest.
func (stage *Stage) AddFS(ctx context.Context, fsys FS, root string, fixity ...digest.Alg) error {
	stage.SetFS(fsys, root)
	if err := stage.checkFSConfig(); err != nil {
		return err
	}
	conc := stage.Concurrency
	if conc < 1 {
		conc = runtime.NumCPU()
	}
	algs := append([]digest.Alg{stage.Alg}, fixity...)
	setup := func(addfn func(name string, algs ...digest.Alg) error) error {
		eachFileFn := func(name string) error {
			return addfn(name)
		}
		return Files(ctx, stage.FS, Dir(stage.Root), eachFileFn)
	}
	// digest result: add results to the stage
	results := func(name string, result digest.Set, err error) error {
		if err != nil {
			return err
		}
		if stage.Root != "." {
			// Trim name so it's relative to root.
			name = strings.TrimPrefix(name, stage.Root+"/")
		}
		return stage.UnsafeAddPath(name, result)
	}
	// checksum file opener uses the stage FS
	open := func(name string) (io.Reader, error) {
		return stage.OpenFile(ctx, name)
	}
	// append required options
	opts := []checksum.Option{
		checksum.WithOpenFunc(open),
		checksum.WithAlgs(algs...),
		checksum.WithNumGos(conc),
	}
	// run checksum
	if err := checksum.Run(ctx, setup, results, opts...); err != nil {
		return fmt.Errorf("while staging root '%s': %w ", root, err)
	}
	return nil
}

// AddPath digests the file identified with name and adds the path to the stage.
// The path name and the associated digest are added to both the stage state and
// its manifest. The file is digested using the stage's primary digest algorith
// and any additional algorithms given by 'fixity'.
func (stage *Stage) AddPath(ctx context.Context, name string, fixity ...digest.Alg) error {
	if err := stage.checkFSConfig(); err != nil {
		return err
	}
	fullName := path.Join(stage.Root, name)
	f, err := stage.OpenFile(ctx, fullName)
	if err != nil {
		return err
	}
	defer f.Close()
	digester := digest.NewDigester(append(fixity, stage.Alg)...)
	if _, err := digester.ReadFrom(f); err != nil {
		return fmt.Errorf("during digest of '%s': %w", fullName, err)
	}
	return stage.UnsafeAddPath(name, digester.Sums())
}

// RemovePath removes the logical path from the stage
func (stage *Stage) RemovePath(n string) error {
	if stage.state == nil {
		stage.state = pathtree.NewDir[string]()
	}
	if _, err := stage.state.Remove(n); err != nil {
		return err
	}
	stage.state.RemoveEmptyDirs()
	return nil
}

// RenamePath renames path src to dst in the stage root. If dst exists, it is
// over-written.
func (stage *Stage) RenamePath(src, dst string) error {
	if stage.state == nil {
		stage.state = pathtree.NewDir[string]()
	}
	return stage.state.Rename(src, dst)
}

// State returns a digest map representing the Stage's logical state.
func (stage Stage) State() *digest.Map {
	if stage.state == nil {
		return &digest.Map{}
	}
	maker := &digest.MapMaker{}
	walkFn := func(p string, n *pathtree.Node[string]) error {
		if n.IsDir() {
			return nil
		}
		if err := maker.Add(n.Val, p); err != nil {
			return err
		}
		return nil
	}
	if err := pathtree.Walk(stage.state, walkFn); err != nil {
		// an error here represents a bug and
		// it should be addressed in testing.
		err = fmt.Errorf("while exporting stage state: %w", err)
		panic(err)
	}
	return maker.Map()
}

// GetStateDigest returns the digest associated with the the logical path in the
// stage state. If the path isn't present as a file in the stage state, an empty
// string is returned.
func (stage Stage) GetStateDigest(lgcPath string) string {
	if stage.state == nil {
		return ""
	}
	node, _ := stage.state.Get(lgcPath)
	if node == nil {
		return ""
	}
	return node.Val
}

// SetStateDigest sets the digest for the logical path to dig replacing the
// previous value if one exists. An error is returned if the logical path exists
// in the state state as a directory or if a parent directory of the logical
// path exists as a file.
func (stage *Stage) SetStateDigest(lgcPath, dig string) error {
	if stage.state == nil {
		stage.state = pathtree.NewDir[string]()
	}
	node, err := stage.state.Get(lgcPath)
	if err == nil && node.IsDir() {
		// TODO: unified type for path conflict error
		return fmt.Errorf("can't add '%s' because it was previously added as a directory", lgcPath)
	}
	return stage.state.SetFile(lgcPath, dig)
}

// GetContent returns the staged content paths for the given
// digest.
func (stage Stage) GetContent(dig string) []string {
	if stage.manifest == nil {
		return nil
	}
	return append([]string{}, stage.manifest[dig].paths...)
}

// GetFixity returns altnerate digest values for the content with the primary
// digest value dig
func (stage Stage) GetFixity(dig string) digest.Set {
	fix := stage.manifest[dig].fixity
	set := make(digest.Set, len(fix))
	for k, v := range fix {
		set[k] = v
	}
	return set
}

// UnsafeAddPath adds name to the stage as both a logical path and a content
// path and associates name with the digests in the digest.Set. digests must
// include an entry with the stage's default digest algorithm. It is unsafe
// because neither the digest or the existence of the file are confirmed.
func (stage *Stage) UnsafeAddPath(name string, digests digest.Set) error {
	return stage.UnsafeAddPathAs(name, name, digests)
}

// UnsafeAddPathAs adds a logical path to the stage and the content path to the
// stage manifest. It is unsafe because neither the digest or the existence of
// the file are confirmed.
func (stage *Stage) UnsafeAddPathAs(content string, logical string, digests digest.Set) error {
	dig, fixity, err := splitDigests(digests, stage.Alg)
	if err != nil {
		return err
	}
	if logical != "" {
		if err := stage.SetStateDigest(logical, dig); err != nil {
			return err
		}
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
func (stage *Stage) UnsafeSetManifestFixty(manifest digest.Map, fixity map[string]*digest.Map) error {
	newContents := stageManifest{}
	err := manifest.EachPath(func(name, dig string) error {
		altDigests := digest.Set{}
		for alg, dmap := range fixity {
			if fixDig := dmap.GetDigest(name); fixDig != "" {
				altDigests[alg] = fixDig
			}
		}
		return newContents.add(dig, name, altDigests)
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
	if stage.Alg == nil {
		return errors.New("stage's digest algorithm is not set")
	}
	return nil
}

// stageManifest is a maps digest values to stageEntries.
type stageManifest map[string]stageEntry

func (man stageManifest) add(dig, name string, fixity digest.Set) error {
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
	paths  []string   // content paths relative to Root in FS
	fixity digest.Set // additional digests associate with paths
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

func (entry *stageEntry) addFixity(fixity digest.Set) {
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

func splitDigests(set digest.Set, alg digest.Alg) (string, digest.Set, error) {
	newSet := digest.Set{}
	dig := ""
	for setAlg, setVal := range set {
		if setAlg == alg.ID() {
			dig = setVal
			continue
		}
		newSet[setAlg] = setVal
	}
	if dig == "" {
		return "", nil, fmt.Errorf("missing %s value", alg.ID())
	}
	return dig, newSet, nil
}
