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
// to commit.
type Stage struct {
	FS              // FS for adding new content to the stage
	Root string     // base directory for all content
	Alg  digest.Alg // Primary digest algorith (sha512 or sha256)

	contents map[string]stageEntry  // map[digest]entry
	state    *pathtree.Node[string] // mutable directory structure
}

type stageEntry struct {
	paths  []string   // content paths relative to Root in FS
	fixity digest.Set // additional digests associate with paths
}

// NewStage creates a new stage with alg as its digest algorithm, init as it its
// initial state and fsys as it backing FS for new content. To initialize an
// empty stage, use an empty digest.Map. If the backing FS is nil, new content
// cannot be added to the stage. The returned stage's Root is set to ".". An
// error is only returned if init is an invalid digest.Map.
func NewStage(alg digest.Alg, init digest.Map, fsys FS) (*Stage, error) {
	stage := &Stage{
		Alg:      alg,
		FS:       fsys,
		Root:     ".",
		contents: map[string]stageEntry{},
	}
	if err := stage.SetState(init); err != nil {
		return nil, err
	}
	return stage, nil
}

// AddPath digests the file identified with name and adds the path to the stage.
// The path name and the associated digest are added to both the stage state and
// its manifest. The file is digested using the stage's primary digest algorith
// and any additional algorithms given by 'fixity'.
func (stage *Stage) AddPath(ctx context.Context, name string, fixity ...digest.Alg) error {
	if err := stage.checkConfig(); err != nil {
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

// AddRoot adds all files in the Stage Root and its subdirectories to the stage.
// Contents are digested using the stage digest algorithm and optional fixity
// algorithms. Paths are added to the stage root with the path relative to the
// Root. If a path was previously added to the stage state, the previous digest
// value associated with path is replaced. However, if a path was previously
// added to the stage manifest (e.g., with UnsafeAdd...) using a different
// digest, AddRoot will fail with an error.
func (stage *Stage) AddRoot(ctx context.Context, root string, fixity ...digest.Alg) error {
	stage.Root = path.Clean(root)
	if err := stage.checkConfig(); err != nil {
		return err
	}
	algs := append([]digest.Alg{stage.Alg}, fixity...)
	setup := func(addfn func(name string, algs ...digest.Alg) error) error {
		eachFileFn := func(name string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return addfn(name)
		}
		return EachFile(ctx, stage.FS, stage.Root, eachFileFn)
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
		checksum.WithNumGos(runtime.NumCPU()), // TODO: make this setable
	}
	// run checksum
	if err := checksum.Run(ctx, setup, results, opts...); err != nil {
		return fmt.Errorf("while staging root '%s': %w ", root, err)
	}
	return nil
}

func (stage *Stage) UnsafeAddPath(name string, digests digest.Set) error {
	return stage.UnsafeAddPathAs(name, name, digests)
}

func (stage *Stage) UnsafeAddPathAs(content string, logical string, digests digest.Set) error {
	dig, fixity, err := splitDigests(digests, stage.Alg)
	if err != nil {
		return err
	}
	if logical != "" {
		if err := stage.addToState(dig, logical); err != nil {
			return err
		}
	}
	if content != "" {
		if err := stage.addToManifest(dig, content, fixity); err != nil {
			return err
		}
	}
	return nil
}

func (stage *Stage) SetState(state digest.Map) error {
	if err := state.Valid(); err != nil {
		return err
	}
	newState := pathtree.NewDir[string]()
	state.EachPath(func(n, d string) error {
		return newState.SetFile(n, d)
	})
	stage.state = newState
	return nil
}

func (stage *Stage) UnsafeSetManifest(manifest digest.Map) {
	manifest.EachPath(func(name, digs string) error {
		return stage.addToManifest(digs, name, nil)
	})
}

// Remove removes the logical path from the stage
func (stage *Stage) Remove(n string) error {
	if stage.state == nil {
		stage.state = pathtree.NewDir[string]()
	}
	if _, err := stage.state.Remove(n); err != nil {
		return err
	}
	stage.state.RemoveEmptyDirs()
	return nil
}

// Rename renames path src to dst in the stage root. If dst exists, it is
// over-written.
func (stage *Stage) Rename(src, dst string) error {
	if stage.state == nil {
		stage.state = pathtree.NewDir[string]()
	}
	return stage.state.Rename(src, dst)
}

// State
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
		panic(err)
	}
	return maker.Map()
}

func (stage Stage) Manifest() *digest.Map {
	// FIXME: shouldn't regenerate State every time
	state := stage.State()
	maker := digest.MapMaker{}
	for d, entry := range stage.contents {
		if state.HasDigest(d) {
			for _, p := range entry.paths {
				if err := maker.Add(d, p); err != nil {
					panic(err)
				}
			}
		}
	}
	return maker.Map()
}

func (stage Stage) ContentPaths(dig string) []string {
	if stage.contents == nil {
		return nil
	}
	return append([]string{}, stage.contents[dig].paths...)
}

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

func (stage *Stage) addToState(dig, name string) error {
	if stage.state == nil {
		stage.state = pathtree.NewDir[string]()
	}
	return stage.state.SetFile(name, dig)
}

func (stage *Stage) addToManifest(dig, name string, fixity digest.Set) error {
	if stage.contents == nil {
		stage.contents = map[string]stageEntry{}
	}
	// // path exists under a different digest?
	// for d, e := range stageFS.contents {
	// 	if d == dgst {
	// 		continue
	// 	}
	// 	for _, p := range e.paths {
	// 		if p == path {
	// 			return fmt.Errorf("the path '%s' was previously assigned to a different digest value", path)
	// 		}
	// 	}
	// }
	entry := stage.contents[dig]
	entry.addPath(name)
	entry.addFixity(fixity)
	stage.contents[dig] = entry
	return nil
}

func (stage *Stage) checkConfig() error {
	if stage.Root == "" {
		stage.Root = "."
	}
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
	id := alg.ID()
	dig := set[id]
	if dig == "" {
		return "", nil, fmt.Errorf("missing %s value", id)
	}
	delete(set, id)
	return dig, set, nil
}
