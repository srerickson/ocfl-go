package ocflv1

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/validation"
	"golang.org/x/exp/slices"
)

var (
	ErrOCFLVersion        = errors.New("unsupported OCFL version")
	ErrInventoryNotExist  = fmt.Errorf("missing inventory file: %w", fs.ErrNotExist)
	ErrInvSidecarContents = errors.New("invalid inventory sidecar contents")
	ErrInvSidecarChecksum = errors.New("inventory digest doesn't match expected value from sidecar file")
	ErrDigestAlg          = errors.New("invalid digest algorithm")
	ErrObjRootStructure   = errors.New("object includes invalid files or directories")
)

func OpenObject(ctx context.Context, fsys ocfl.FS, dir string) (*FunObject, error) {
	if !fs.ValidPath(dir) {
		return nil, &fs.PathError{
			Op:   "open",
			Path: dir,
			Err:  fs.ErrInvalid,
		}
	}
	var inv *Inventory
	invFile, err := fsys.OpenFile(ctx, path.Join(dir, inventoryFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		// additional checks in inventory doesn't exist?
	}
	if invFile != nil {
		defer func() {
			if closeErr := invFile.Close(); closeErr != nil {
				err = errors.Join(err, invFile.Close())
			}
		}()
		bytes, err := io.ReadAll(invFile)
		if err != nil {
			return nil, err
		}
		inv = &Inventory{}
		if err := json.Unmarshal(bytes, inv); err != nil {
			return nil, err
		}
	}
	// inventory may be nil
	obj := &FunObject{fs: fsys, path: dir, inv: inv}
	return obj, nil
}

// TODO: This is the new object
type FunObject struct {
	fs   ocfl.FS
	path string
	inv  *Inventory
}

func (o FunObject) Close() error { return nil }

func (o *FunObject) FS() ocfl.FS  { return o.fs }
func (o *FunObject) Exists() bool { return o.inv != nil }

func (o *FunObject) Inventory() ocfl.Inventory {
	if o.inv == nil {
		return nil
	}
	return &inventory{inv: *o.inv}
}

func (o *FunObject) Validate(ctx context.Context, opts *ocfl.Validation) *ocfl.ValidationResult {
	_, r := ValidateObject(ctx, o.fs, o.path)
	result := &ocfl.ValidationResult{}
	result.Fatal = multierror.Append(result.Fatal, r.Fatal()...)
	result.Warning = multierror.Append(result.Warning, r.Warn()...)
	return result
}

func (o *FunObject) VersionFS(ctx context.Context, i int) ocfl.FSCloser {
	ver := o.inv.Version(i)
	if ver == nil {
		return nil
	}
	// FIXME: This is a hacky way to wmake versionFS replicates the filemode of
	// the undering FS. Open a random content file to get the file mode used by
	// the underlying FS.
	regfileType := fs.FileMode(0)
	for _, paths := range o.inv.Manifest {
		if len(paths) < 1 {
			break
		}
		f, err := o.fs.OpenFile(ctx, path.Join(o.path, paths[0]))
		if err != nil {
			return nil
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return nil
		}
		regfileType = info.Mode().Type()
		break
	}
	return &versionFS{
		ctx:     ctx,
		obj:     o,
		state:   ver.State,
		created: ver.Created,
		regMode: regfileType,
	}
}

func (o *FunObject) Path() string { return o.path }

// Object represents an existing OCFL v1.x object. Use GetObject() to initialize
// new Objects.
type Object struct {
	*ocfl.ObjectRoot
	Inventory Inventory
}

// GetObject returns an existing oject at dir in fsys. It returns an error if
// dir doesn't exist or doesn't include an object declaration file, or if the
// contents of the root inventory can't be unmarshalled into an Inventory value.
// Neither the object root or the inventory are fully validated.
func GetObject(ctx context.Context, fsys ocfl.FS, dir string) (*Object, error) {
	root, err := ocfl.GetObjectRoot(ctx, fsys, dir)
	if err != nil {
		return nil, err
	}
	if !ocflVerSupported[root.State.Spec] {
		return nil, fmt.Errorf("%s: %w", root.State.Spec, ErrOCFLVersion)
	}
	if !root.State.HasInventory() {
		// what is the best error to use here?
		return nil, ErrInventoryNotExist
	}
	obj := &Object{ObjectRoot: root}
	if err := obj.ReadInventory(ctx); err != nil {
		return nil, err
	}
	return obj, nil
}

// ReadInventory reads and unmarshals the object's existing root inventory into
// obj.Inventory.
func (obj *Object) ReadInventory(ctx context.Context) error {
	var newInv Inventory
	if err := obj.ObjectRoot.UnmarshalInventory(ctx, ".", &newInv); err != nil {
		return err
	}
	obj.Inventory = newInv
	return nil
}

// Validate fully validates the Object. If the object is valid, the Object's inventory
// is updated with the inventory downloaded during validation.
func (obj *Object) Validate(ctx context.Context, opts ...ValidationOption) *validation.Result {
	newObj, r := ValidateObject(ctx, obj.FS, obj.Path, opts...)
	if r.Err() == nil {
		obj.Inventory = newObj.Inventory
	}
	return r
}

// Stage returns an ocfl.Stage based on the specified version index.
func (obj *Object) Stage(i int) (*ocfl.Stage, error) {
	version := obj.Inventory.Version(i)
	if version == nil {
		return nil, ErrVersionNotFound
	}
	state, err := version.State.Normalize()
	if err != nil {
		return nil, err
	}
	return &ocfl.Stage{
		State:           state,
		DigestAlgorithm: obj.Inventory.DigestAlgorithm,
		ContentSource:   obj,
		FixitySource:    obj,
	}, nil
}

// GetContent implements ocfl.ContentSource for Object
func (obj *Object) GetContent(digest string) (ocfl.FS, string) {
	if obj.Inventory.Manifest == nil {
		return nil, ""
	}
	paths := obj.Inventory.Manifest[digest]
	if len(paths) < 1 {
		return nil, ""
	}
	return obj.FS, path.Join(obj.ObjectRoot.Path, paths[0])
}

// GetFixity implements ocfl.FixitySource for Object
func (obj Object) GetFixity(digest string) ocfl.DigestSet {
	return obj.Inventory.GetFixity(digest)
}

// Objects returns a function iterator that yields Objects
// found in dir and its subdirectories
func Objects(ctx context.Context, fsys ocfl.FS, dir string) ObjectSeq {
	return func(yieldObject func(*Object, error) bool) {
		objectRootIter := ocfl.ObjectRoots(ctx, fsys, dir)
		objectRootIter(func(objRoot *ocfl.ObjectRoot, err error) bool {
			var obj *Object
			if objRoot != nil {
				obj = &Object{ObjectRoot: objRoot}
			}
			if err != nil && !yieldObject(obj, err) {
				return false
			}
			return yieldObject(obj, obj.ReadInventory(ctx))
		})
	}
}

type ObjectSeq func(yield func(*Object, error) bool)

type versionFS struct {
	ctx     context.Context
	obj     *FunObject
	state   ocfl.DigestMap
	created time.Time
	regMode fs.FileMode
}

func (vfs versionFS) Close() error { return nil }
func (vfs *versionFS) Open(logical string) (fs.File, error) {
	if !fs.ValidPath(logical) {
		return nil, &fs.PathError{
			Err:  fs.ErrInvalid,
			Op:   "open",
			Path: logical,
		}
	}
	if logical == "." {
		return vfs.openDir(".")
	}

	digest := vfs.state.GetDigest(logical)
	if digest == "" {
		// name doesn't exist in state.
		// try opening as a directory
		return vfs.openDir(logical)
	}

	realNames := vfs.obj.inv.Manifest[digest]
	if len(realNames) < 1 {
		return nil, &fs.PathError{
			Err:  fs.ErrNotExist,
			Op:   "open",
			Path: logical,
		}
	}
	realName := realNames[0]
	if !fs.ValidPath(realName) {
		return nil, &fs.PathError{
			Err:  fs.ErrInvalid,
			Op:   "open",
			Path: logical,
		}
	}
	f, err := vfs.obj.fs.OpenFile(vfs.ctx, path.Join(vfs.obj.path, realName))
	if err != nil {
		err = fmt.Errorf("opening file with logical path %q: %w", logical, err)
		return nil, err
	}
	return f, nil
}

func (vfs *versionFS) openDir(dir string) (fs.File, error) {
	prefix := dir + "/"
	if prefix == "./" {
		prefix = ""
	}
	children := map[string]*vfsDirEntry{}
	for _, paths := range vfs.state {
		for _, p := range paths {
			if !strings.HasPrefix(p, prefix) {
				continue
			}
			name, _, isdir := strings.Cut(strings.TrimPrefix(p, prefix), "/")
			if _, exists := children[name]; exists {
				continue
			}
			entry := &vfsDirEntry{
				name:    name,
				mode:    vfs.regMode,
				created: vfs.created,
				open:    func() (fs.File, error) { return vfs.Open(path.Join(dir, name)) },
			}
			if isdir {
				entry.mode = entry.mode | fs.ModeDir | fs.ModeIrregular
			}
			children[name] = entry
		}
	}

	if len(children) < 1 {
		return nil, &fs.PathError{
			Op:   "open",
			Path: dir,
			Err:  fs.ErrNotExist,
		}
	}

	dirFile := &vfsDirFile{
		name:    dir,
		entries: make([]fs.DirEntry, 0, len(children)),
	}
	for _, entry := range children {
		dirFile.entries = append(dirFile.entries, entry)
	}
	slices.SortFunc(dirFile.entries, func(a, b fs.DirEntry) int {
		return cmp.Compare(a.Name(), b.Name())
	})
	return dirFile, nil
}

type vfsDirEntry struct {
	name    string
	created time.Time
	mode    fs.FileMode
	open    func() (fs.File, error)
}

var _ fs.DirEntry = (*vfsDirEntry)(nil)

func (info *vfsDirEntry) Name() string      { return info.name }
func (info *vfsDirEntry) IsDir() bool       { return info.mode.IsDir() }
func (info *vfsDirEntry) Type() fs.FileMode { return info.mode.Type() }

func (info *vfsDirEntry) Info() (fs.FileInfo, error) {
	f, err := info.open()
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	return stat, errors.Join(err, f.Close())
}

func (info *vfsDirEntry) Size() int64        { return 0 }
func (info *vfsDirEntry) Mode() fs.FileMode  { return info.mode | fs.ModeIrregular }
func (info *vfsDirEntry) ModTime() time.Time { return info.created }
func (info *vfsDirEntry) Sys() any           { return nil }

type vfsDirFile struct {
	name    string
	created time.Time
	entries []fs.DirEntry
	offset  int
}

var _ fs.ReadDirFile = (*vfsDirFile)(nil)

func (dir *vfsDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		entries := dir.entries[dir.offset:]
		dir.offset = len(dir.entries)
		return entries, nil
	}
	if remain := len(dir.entries) - dir.offset; remain < n {
		n = remain
	}
	if n <= 0 {
		return nil, io.EOF
	}
	entries := dir.entries[dir.offset : dir.offset+n]
	dir.offset += n
	return entries, nil
}

func (dir *vfsDirFile) Close() error               { return nil }
func (dir *vfsDirFile) IsDir() bool                { return true }
func (dir *vfsDirFile) Mode() fs.FileMode          { return fs.ModeDir | fs.ModeIrregular }
func (dir *vfsDirFile) ModTime() time.Time         { return dir.created }
func (dir *vfsDirFile) Name() string               { return dir.name }
func (dir *vfsDirFile) Read(_ []byte) (int, error) { return 0, nil }
func (dir *vfsDirFile) Size() int64                { return 0 }
func (dir *vfsDirFile) Stat() (fs.FileInfo, error) { return dir, nil }
func (dir *vfsDirFile) Sys() any                   { return nil }
