package ocfl

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"log/slog"

	"github.com/srerickson/ocfl-go/digest"
)

// Object represents and OCFL Object, typically contained in a Root.
type Object struct {
	// object's storage backend. Must implement WriteFS to commit.
	fs FS
	// path in FS for object root directory
	path string
	// object's root inventory
	inventory Inventory
	// object id used to open the object from the root
	expectID string
	// the object must exist: don't create a new object.
	mustExist bool
	//TODO pointer to object's storage root.
}

// NewObject returns an *Object for managing the OCFL object at path in fsys.
// The object doesn't need to exist when NewObject is called.
func NewObject(ctx context.Context, fsys FS, dir string, opts ...ObjectOption) (*Object, error) {
	if !fs.ValidPath(dir) {
		return nil, fmt.Errorf("invalid object path: %q: %w", dir, fs.ErrInvalid)
	}
	obj := newObject(fsys, dir, opts...)
	// read root inventory: we don't know what OCFL spec it uses.
	inv, err := ReadInventory(ctx, fsys, dir)
	if err != nil {
		var pthError *fs.PathError
		if !errors.As(err, &pthError) {
			return nil, err
		}
		if path.Base(pthError.Path) != inventoryBase {
			// error is not from opening `inventory.json`
			return nil, err
		}
		if !errors.Is(err, fs.ErrNotExist) || obj.mustExist {
			return nil, err
		}
	}
	if inv != nil {
		// check that inventory has expected object ID
		// if the expected object ID is known.
		if obj.expectID != "" && inv.ID() != obj.expectID {
			err := fmt.Errorf("object has unexpected ID: %q; expected: %q", inv.ID(), obj.expectID)
			return nil, err
		}
		obj.setInventory(inv)
		return obj, nil
	}
	// inventory.json doesn't exist: open as uninitialized object. The object
	// root directory must not exist or be an empty directory. Note, the object's
	// ocfl implementation is not set!
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("reading object root directory: %w", err)
		}
	}
	rootState := ParseObjectDir(entries)
	switch {
	case rootState.Empty():
		return obj, nil
	case rootState.HasNamaste():
		return nil, fmt.Errorf("incomplete OCFL object: %s: %w", inventoryBase, fs.ErrNotExist)
	default:
		return nil, fmt.Errorf("directory is not an OCFL object: %w", ErrObjectNamasteNotExist)
	}
}

// create a new *Object with required feilds and apply options
func newObject(fsys FS, dir string, opts ...ObjectOption) *Object {
	obj := &Object{fs: fsys, path: dir}
	for _, optFn := range opts {
		optFn(obj)
	}
	return obj
}

// Commit creates a new object version based on values in commit.
func (obj *Object) Commit(ctx context.Context, commit *Commit) error {
	if _, isWriteFS := obj.FS().(WriteFS); !isWriteFS {
		return errors.New("object's backing file system doesn't support write operations")
	}
	// the OCFL implementation for the new object version
	var useOCFL ocfl
	switch {
	case commit.Spec.Empty():
		switch {
		case !obj.Exists():
			// new object and no ocfl version specified in commit
			useOCFL = defaultOCFL()
		default:
			// use existing object's ocfl version
			var err error
			useOCFL, err = getOCFL(obj.inventory.Spec())
			if err != nil {
				err = fmt.Errorf("object's root inventory has errors: %w", err)
				return &CommitError{Err: err}
			}
		}
		commit.Spec = useOCFL.Spec()
	default:
		var err error
		useOCFL, err = getOCFL(commit.Spec)
		if err != nil {
			return &CommitError{Err: err}
		}
	}
	// set commit's object id if we have an expected id and commit ID isn't set
	if obj.expectID != "" && commit.ID != obj.expectID {
		if commit.ID != "" {
			err := fmt.Errorf("commit includes unexpected object ID: %s; expected: %q", commit.ID, obj.expectID)
			return &CommitError{Err: err}
		}
		commit.ID = obj.expectID
	}
	if err := useOCFL.Commit(ctx, obj, commit); err != nil {
		return err
	}
	return nil
}

// Exists returns true if the object has an existing version.
func (obj *Object) Exists() bool {
	return obj.inventory != nil
}

// ExtensionNames returns the names of directories in the object's
// extensions directory. The ObjectRoot's State is initialized if it is
// nil. If the object root does not include an object declaration, an error
// is returned. If object root does not include an extensions directory both
// return values are nil.
func (obj Object) ExtensionNames(ctx context.Context) ([]string, error) {
	entries, err := obj.FS().ReadDir(ctx, path.Join(obj.path, extensionsDir))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			// if the extensions directory includes non-directory
			// entries, should we return an error?
			continue
		}
		names = append(names, e.Name())
	}
	return names, err
}

// FS returns the FS where object is stored.
func (obj *Object) FS() FS {
	return obj.fs
}

// Inventory returns the object's Inventory if it exists. If the object
// doesn't exist, it returns nil.
func (obj *Object) Inventory() Inventory {
	return obj.inventory
}

// Path returns the Object's path relative to its FS.
func (obj *Object) Path() string {
	return obj.path
}

// OpenVersion returns an ObjectVersionFS for the version with the given
// index (1...HEAD). If i is 0, the most recent version is used.
func (obj *Object) OpenVersion(ctx context.Context, i int) (*ObjectVersionFS, error) {
	if !obj.Exists() {
		return nil, ErrNamasteNotExist
	}
	inv := obj.Inventory()
	if inv == nil {
		// FIXME; better error
		return nil, errors.New("object is missing an inventory")
	}
	if i == 0 {
		i = inv.Head().num
	}
	ver := inv.Version(i)
	if ver == nil {
		// FIXME; better error
		return nil, errors.New("version not found")
	}
	ioFS := obj.VersionFS(ctx, i)
	if ioFS == nil {
		// FIXME; better error
		return nil, errors.New("version not found")
	}
	vfs := &ObjectVersionFS{
		fsys: ioFS,
		ver:  ver,
		num:  i,
		inv:  inv,
	}
	return vfs, nil
}

func (obj *Object) setInventory(inv Inventory) {
	obj.inventory = inv
}

// ValidateObject fully validates the OCFL Object at dir in fsys
func ValidateObject(ctx context.Context, fsys FS, dir string, opts ...ObjectValidationOption) *ObjectValidation {
	v := newObjectValidation(fsys, dir, opts...)
	if !fs.ValidPath(dir) {
		err := fmt.Errorf("invalid object path: %q: %w", dir, fs.ErrInvalid)
		v.AddFatal(err)
		return v
	}
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		v.AddFatal(err)
		return v
	}
	state := ParseObjectDir(entries)
	impl, err := getOCFL(state.Spec)
	if err != nil {
		// unknown OCFL version
		v.AddFatal(err)
		return v
	}
	if err := impl.ValidateObjectRoot(ctx, v, state); err != nil {
		return v
	}
	// validate versions using previous specs
	versionOCFL := lowestOCFL()
	var prevInv Inventory
	for _, vnum := range state.VersionDirs.Head().Lineage() {
		versionDir := path.Join(dir, vnum.String())
		versionInv, err := ReadInventory(ctx, fsys, versionDir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			v.AddFatal(fmt.Errorf("reading %s/inventory.json: %w", vnum, err))
			continue
		}
		if versionInv != nil {
			versionOCFL = mustGetOCFL(versionInv.Spec())
		}
		versionOCFL.ValidateObjectVersion(ctx, v, vnum, versionInv, prevInv)
		prevInv = versionInv
	}
	impl.ValidateObjectContent(ctx, v)
	return v
}

// Commit represents an update to object.
type Commit struct {
	ID      string // required for new objects in storage roots without a layout.
	Stage   *Stage // required
	Message string // required
	User    User   // required

	// advanced options
	Created         time.Time // time.Now is used, if not set
	Spec            Spec      // OCFL specification version for the new object version
	NewHEAD         int       // enforces new object version number
	AllowUnchanged  bool
	ContentPathFunc RemapFunc

	Logger *slog.Logger
}

// Commit error wraps an error from a commit.
type CommitError struct {
	Err error // The wrapped error

	// Dirty indicates the object may be incomplete or invalid as a result of
	// the error.
	Dirty bool
}

func (c CommitError) Error() string {
	return c.Err.Error()
}

func (c CommitError) Unwrap() error {
	return c.Err
}

type ObjectVersionFS struct {
	fsys fs.FS
	ver  ObjectVersion
	inv  Inventory
	num  int
}

func (vfs *ObjectVersionFS) GetContent(digest string) (FS, string) {
	dm := vfs.State()
	if dm == nil {
		return nil, ""
	}
	pths := dm[digest]
	if len(pths) < 1 {
		return nil, ""
	}
	return &ioFS{FS: vfs.fsys}, pths[0]
}

func (vfs *ObjectVersionFS) Close() error {
	if closer, isCloser := vfs.fsys.(io.Closer); isCloser {
		return closer.Close()
	}
	return nil
}
func (vfs *ObjectVersionFS) Created() time.Time                { return vfs.ver.Created() }
func (vfs *ObjectVersionFS) DigestAlgorithm() digest.Algorithm { return vfs.inv.DigestAlgorithm() }
func (vfs *ObjectVersionFS) State() DigestMap                  { return vfs.ver.State() }
func (vfs *ObjectVersionFS) Message() string                   { return vfs.ver.Message() }
func (vfs *ObjectVersionFS) Num() int                          { return vfs.num }
func (vfs *ObjectVersionFS) Open(name string) (fs.File, error) { return vfs.fsys.Open(name) }
func (vfs *ObjectVersionFS) User() *User                       { return vfs.ver.User() }

func (vfs *ObjectVersionFS) Stage() *Stage {
	return &Stage{
		State:           vfs.State().Clone(),
		DigestAlgorithm: vfs.inv.DigestAlgorithm(),
		FixitySource:    vfs.inv,
		ContentSource:   vfs,
	}
}

// ObjectOptions are used to configure the behavior of NewObject()
type ObjectOption func(*Object)

// ObjectMustExists requires the object to exist
func ObjectMustExist() ObjectOption {
	return func(o *Object) {
		o.mustExist = true
	}
}

func objectExpectedID(id string) ObjectOption {
	return func(o *Object) {
		o.expectID = id
	}
}

func (o *Object) VersionFS(ctx context.Context, i int) fs.FS {
	ver := o.inventory.Version(i)
	if ver == nil {
		return nil
	}
	// FIXME: This is a hack to make versionFS replicates the filemode of
	// the undering FS. Open a random content file to get the file mode used by
	// the underlying FS.
	regfileType := fs.FileMode(0)
	for _, paths := range o.inventory.Manifest() {
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
		paths:   ver.State().PathMap(),
		created: ver.Created(),
		regMode: regfileType,
	}
}

type versionFS struct {
	ctx     context.Context
	obj     *Object
	paths   PathMap
	created time.Time
	regMode fs.FileMode
}

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
	digest := vfs.paths[logical]
	if digest == "" {
		// name doesn't exist in state.
		// try opening as a directory
		return vfs.openDir(logical)
	}

	realNames := vfs.obj.inventory.Manifest()[digest]
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
	for p := range vfs.paths {
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
