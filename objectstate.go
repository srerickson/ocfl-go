package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sync"
	"time"

	"github.com/srerickson/ocfl-go/internal/pathtree"
)

const (
	OBJECTSTATE_DEFAULT_FILEMODE fs.FileMode = 0440
	OBJECTSTATE_DEFAULT_DIRMODE              = 0550 | fs.ModeDir
)

// ObjectState encapsulates a set of logical content (i.e., an object version
// state) and its mapping to specific content paths in Manifest.
type ObjectState struct {
	DigestMap           // digests / logical paths
	Manifest  DigestMap // digests / content paths
	Alg       string    // algorith used for digests
	User      *User     // user who created object state
	Created   time.Time // object state created at
	Message   string    // message associated with object state
	VNum      VNum      // version represented by the object state
	Head      VNum      // object's head version
	Spec      Spec      // OCFL spec for the object version for the state
}

// User is a generic user information struct
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

// ObjectStateFS implements FS for the logical contents of the ObjectState
type ObjectStateFS struct {
	ObjectState
	// OpenContentFile opens a content file using the path from the object state
	// manifest.
	OpenContentFile func(ctx context.Context, name string) (fs.File, error)

	buildLock sync.Mutex
	index     *pathtree.Node[string] // logical directory structure
}

// OpenFile is used to access files in the Objects State by their logical paths
func (state *ObjectStateFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if err := state.buildIndex(); err != nil {
		return nil, wrapFSPathError("openfile", name, err)
	}
	// node value is the content path corresponding to the logical path
	node, err := state.index.Get(name)
	if err != nil {
		return nil, wrapFSPathError("openfile", name, err)
	}
	if node.IsDir() {
		return nil, wrapFSPathError("openfile", name, ErrNotFile)
	}
	f, err := state.OpenContentFile(ctx, node.Val)
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
func (state *ObjectStateFS) ReadDir(ctx context.Context, dirPath string) ([]fs.DirEntry, error) {
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

// objStateFileInfo implements fs.FileInfo
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

func (state *ObjectStateFS) buildIndex() (err error) {
	state.buildLock.Lock()
	defer state.buildLock.Unlock()
	if state.index == nil {
		state.index = pathtree.NewDir[string]()

		state.DigestMap.EachPath(func(name, dig string) bool {
			realPaths := state.Manifest.DigestPaths(dig)
			if len(realPaths) == 0 {
				err = fmt.Errorf("missing content paths for digest '%s'", name)
				return false
			}
			err = state.index.SetFile(name, realPaths[0])
			return err == nil
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
