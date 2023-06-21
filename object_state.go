package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"path"
	"sync"
	"time"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/pathtree"
)

const (
	OBJECTSTATE_DEFAULT_FILEMODE fs.FileMode = 0440
	OBJECTSTATE_DEFAULT_DIRMODE              = 0550 | fs.ModeDir
)

// ObjectState encapsulates a set of logical content (i.e., an object version
// state) and its mapping to specific content in fs (i.e., paths in a manifest that
// can be read from an FS)
type ObjectState struct {
	digest.Map            // digests / logical paths
	Manifest   digest.Map // digests / content paths
	Alg        digest.Alg // algorith used for digests
	FS         FS         // FS for content paths
	Root       string     // content paths are relative to Root
	Created    time.Time
	Message    string

	buildLock sync.Mutex
	index     *pathtree.Node[string] // logical directory structure
}

// AsState returns a Stage based on the ObjectState. It panics if the object
// state is invalid. If asFS is true, the state returned Stage also uses the
// state as it's backing FS and the stage manifest will match the state. This is
// useful in cases where the Stage will be used to create or update a different
// than the one the Object State is derived from.
func (state *ObjectState) AsStage(asFS bool) (*Stage, error) {
	stage, err := NewStage(state.Alg, state.Map, nil)
	if err != nil {
		err := fmt.Errorf("building stage from invalid object state: %w", err)
		panic(err)
	}
	if asFS {
		stage.FS = state
		stage.Root = "."
		stage.UnsafeSetManifest(state.Map)
	}
	return stage, nil
}

// OpenFile is used to access files in the Objects State by their logical paths
func (state *ObjectState) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if err := state.buildIndex(); err != nil {
		return nil, wrapFSPathError("openfile", name, err)
	}
	node, err := state.index.Get(name)
	if err != nil {
		return nil, wrapFSPathError("openfile", name, err)
	}
	if node.IsDir() {
		log.Println(name)
		return nil, wrapFSPathError("openfile", name, ErrNotFile)
	}
	f, err := state.FS.OpenFile(ctx, path.Join(state.Root, node.Val))
	if err != nil {
		return nil, wrapFSPathError("openfile", name, err)
	}
	return &objStateFile{
		File:    f,
		name:    path.Base(name),
		modtime: state.Created,
	}, nil
}

// OpenDir is used to access directories in the Objects State by their logical paths
func (state *ObjectState) ReadDir(ctx context.Context, dirPath string) ([]fs.DirEntry, error) {
	if err := state.buildIndex(); err != nil {
		return nil, wrapFSPathError("opendir", dirPath, err)
	}
	dirNode, err := state.index.Get(dirPath)
	if err != nil {
		return nil, wrapFSPathError("opendir", dirPath, err)
	}
	if !dirNode.IsDir() {
		// FIXME: need ErrNotDir?
		return nil, wrapFSPathError("opendir", dirPath, errors.New("not a directory"))
	}
	children := dirNode.DirEntries()
	dirEntries := make([]fs.DirEntry, len(children))
	for i, child := range children {
		dirEntry := &objStateDirEntry{
			name:    child.Name(),
			isdir:   child.IsDir(),
			modtime: state.Created,
		}
		// set stat for file entries
		if !dirEntry.isdir {
			filePath := path.Join(dirPath, child.Name())
			dirEntry.stat = func() (fs.FileInfo, error) {
				f, err := state.OpenFile(ctx, filePath)
				if err != nil {
					return nil, err
				}
				defer f.Close()
				return f.Stat()
			}
		}
		dirEntries[i] = dirEntry
	}
	return dirEntries, nil
}

// objStateFile is use to provide the logical name
// used with OpenFile to the fs.FileInfo returned by Stat()
type objStateFile struct {
	fs.File           // file with content
	name    string    // logical name
	modtime time.Time // object state created
}

func (file objStateFile) Stat() (fs.FileInfo, error) {
	baseInfo, err := file.File.Stat()
	if err != nil {
		return nil, err
	}
	return objStateFileInfo{
		name:     file.name,
		baseInfo: baseInfo,
		modtime:  file.modtime,
		mode:     OBJECTSTATE_DEFAULT_FILEMODE,
	}, nil
}

// result from ReadDir()
type objStateDirEntry struct {
	name    string // logical name
	isdir   bool
	modtime time.Time // from objec state created
	stat    func() (fs.FileInfo, error)
}

func (entry objStateDirEntry) Name() string { return entry.name }
func (entry objStateDirEntry) IsDir() bool  { return entry.isdir }
func (entry objStateDirEntry) Type() fs.FileMode {
	if entry.isdir {
		return fs.ModeDir
	}
	return 0
}
func (entry *objStateDirEntry) Info() (fs.FileInfo, error) {
	// stat must be set for all files
	if !entry.isdir {
		return entry.stat()
	}
	// otherwise, return generic directory info
	return objStateFileInfo{
		name:    entry.name,
		modtime: entry.modtime,
		mode:    OBJECTSTATE_DEFAULT_DIRMODE,
	}, nil
}

// objStateFileInfo implementes fs.FileInfo
type objStateFileInfo struct {
	name     string // logical name from OpenFile/OpenDir
	modtime  time.Time
	mode     fs.FileMode
	baseInfo fs.FileInfo // FileInfo from underlying FS
}

func (info objStateFileInfo) Name() string       { return info.name }
func (info objStateFileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info objStateFileInfo) ModTime() time.Time { return info.modtime }
func (info objStateFileInfo) Mode() fs.FileMode  { return info.mode }

func (info objStateFileInfo) Size() int64 {
	if info.baseInfo != nil {
		return info.baseInfo.Size()
	}
	return 0
}

func (info objStateFileInfo) Sys() any {
	if info.baseInfo != nil {
		return info.baseInfo.Sys()
	}
	return nil
}

func (state *ObjectState) buildIndex() (err error) {
	state.buildLock.Lock()
	defer state.buildLock.Unlock()
	if state.index == nil {
		state.index = pathtree.NewDir[string]()
		err = state.Map.EachPath(func(name, dig string) error {
			realPaths := state.Manifest.DigestPaths(dig)
			if len(realPaths) == 0 {
				return fmt.Errorf("missing content paths for digest '%s'", name)
			}
			return state.index.SetFile(name, realPaths[0])
		})
	}
	return
}

func wrapFSPathError(op string, name string, err error) error {
	if errors.Is(err, pathtree.ErrInvalidPath) {
		return &fs.PathError{
			Op:   op,
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	if errors.Is(err, pathtree.ErrNotFound) {
		return &fs.PathError{
			Op:   op,
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}
	return &fs.PathError{
		Op:   op,
		Path: name,
		Err:  err,
	}
}
