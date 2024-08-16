package ocflv1

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"golang.org/x/exp/slices"
)

var (
	ErrOCFLVersion      = errors.New("unsupported OCFL version")
	ErrObjRootStructure = errors.New("object includes invalid files or directories")
)

// ReadObject implements ocfl.ReadObject for OCFL v1.x objects
type ReadObject struct {
	fs   ocfl.FS
	path string
	inv  *Inventory
}

func (o *ReadObject) FS() ocfl.FS { return o.fs }

func (o *ReadObject) Inventory() ocfl.ReadInventory {
	if o.inv == nil {
		return nil
	}
	return &readInventory{raw: *o.inv}
}

func (o *ReadObject) ValidateContent(ctx context.Context, v *ocfl.ObjectValidation) {
	ocflV := o.inv.Type.Spec
	v.MissingContent()(func(name string) bool {
		err := fmt.Errorf("missing content: %s", name)
		v.AddFatal(ec(err, codes.E092(ocflV)))
		return true
	})
	v.UnexpectedContent()(func(name string) bool {
		err := fmt.Errorf("unexpected content: %s", name)
		v.AddFatal(ec(err, codes.E023(ocflV)))
		return true
	})
	if !v.SkipDigests() {
		v.ExistingContentDigests()(func(name string, digests ocfl.DigestSet) bool {
			// TODO concurrent digests
			f, err := o.fs.OpenFile(ctx, path.Join(o.path, name))
			if err != nil {
				err = fmt.Errorf("unexpected error while validating digests: %w", err)
				v.AddFatal(err)
				return true
			}
			defer func() {
				if closeErr := f.Close(); closeErr != nil {
					v.AddFatal(closeErr)
				}
			}()
			if err := digests.Validate(f); err != nil {
				err = fmt.Errorf("validating digests for %q: %w", name, err)
				v.AddFatal(ec(err, codes.E093(ocflV)))
			}
			return true
		})
	}
}

func (o *ReadObject) ValidateRoot(ctx context.Context, state *ocfl.ObjectState, vldr *ocfl.ObjectValidation) {
	if err := o.validateDeclaration(ctx); err != nil {
		vldr.AddFatal(err)
	}
	vldr.PrefixAdd("root contents", o.validateRootState(state))
	vldr.PrefixAdd("inventory.json", o.inv.Validate())
	vldr.PrefixAdd("extensions directory", o.validateExtensionsDir(ctx))
	if err := vldr.AddInventoryDigests(o.Inventory()); err != nil {
		vldr.AddFatal(err)
	}
}

func (o ReadObject) validateRootState(state *ocfl.ObjectState) *ocfl.Validation {
	ocflV := o.inv.Type.Spec
	v := &ocfl.Validation{}
	for _, name := range state.Invalid {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		v.AddFatal(ec(err, codes.E001(ocflV)))
	}
	if !state.HasInventory() {
		err := fmt.Errorf(`root inventory.json: %w`, fs.ErrNotExist)
		v.AddFatal(ec(err, codes.E063(ocflV)))
	}
	if !state.HasSidecar() {
		err := fmt.Errorf(`root inventory.json sidecar: %w`, fs.ErrNotExist)
		v.AddFatal(ec(err, codes.E058(ocflV)))
	}
	err := state.VersionDirs.Valid()
	if err != nil {
		if errors.Is(err, ocfl.ErrVerEmpty) {
			err = ec(err, codes.E008(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumPadding) {
			err = ec(err, codes.E011(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumMissing) {
			err = ec(err, codes.E010(ocflV))
		}
		v.AddFatal(err)
	}
	if err == nil && state.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		v.AddWarn(ec(err, codes.W001(ocflV)))
	}
	if vdirHead := state.VersionDirs.Head().Num(); vdirHead > o.inv.Head.Num() {
		err := errors.New("version directories don't reflect versions in inventory.json")
		v.AddFatal(ec(err, codes.E046(ocflV)))
	}
	return v
}

func (o *ReadObject) validateDeclaration(ctx context.Context) error {
	ocflV := o.inv.Type.Spec
	decl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: ocflV}
	name := path.Join(o.path, decl.Name())
	err := ocfl.ValidateNamaste(ctx, o.fs, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("%s: %w", name, ocfl.ErrObjectNamasteNotExist)
		}
		return err
	}
	return nil
}

func (o *ReadObject) validateExtensionsDir(ctx context.Context) *ocfl.Validation {
	ocflV := o.inv.Type.Spec
	v := &ocfl.Validation{}
	extDir := path.Join(o.path, extensionsDir)
	items, err := o.fs.ReadDir(ctx, extDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		v.AddFatal(err)
		return v
	}
	for _, i := range items {
		if !i.IsDir() {
			err := fmt.Errorf(`invalid file: %s`, i.Name())
			v.AddFatal(ec(err, codes.E067(ocflV)))
			continue
		}
		_, err := extension.Get(i.Name())
		if err != nil {
			// unknow extension
			err := fmt.Errorf("%w: %s", err, i.Name())
			v.AddWarn(ec(err, codes.W013(ocflV)))
		}
	}
	return v
}

func (o *ReadObject) VersionFS(ctx context.Context, i int) fs.FS {
	ver := o.inv.Version(i)
	if ver == nil {
		return nil
	}
	// FIXME: This is a hack to make versionFS replicates the filemode of
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
		paths:   ver.State.PathMap(),
		created: ver.Created,
		regMode: regfileType,
	}
}

func (o *ReadObject) Path() string { return o.path }

type versionFS struct {
	ctx     context.Context
	obj     *ReadObject
	paths   ocfl.PathMap
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
