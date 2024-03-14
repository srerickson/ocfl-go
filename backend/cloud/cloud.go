package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/internal/walkdirs"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
)

var ErrNotDir = fmt.Errorf("not a directory")

// FS is a generic backend for cloud storage backends using a blob.Bucket
type FS struct {
	*blob.Bucket
	log        *slog.Logger
	writerOpts *blob.WriterOptions
	readerOpts *blob.ReaderOptions

	// Experimental config
	ObjectRootsUseWalkDirs bool
	ObjectRootWalkDirsGos  int
}

var _ ocfl.WriteFS = (*FS)(nil)

type fsOption func(*FS)

func NewFS(b *blob.Bucket, opts ...fsOption) *FS {
	fsys := &FS{
		Bucket: b,
	}
	for _, opt := range opts {
		opt(fsys)
	}
	return fsys
}

func WithLogger(l *slog.Logger) fsOption {
	return func(fsys *FS) {
		fsys.log = l
	}
}

func (fsys *FS) WriterOptions(opts *blob.WriterOptions) *FS {
	return &FS{
		Bucket:     fsys.Bucket,
		log:        fsys.log,
		writerOpts: opts,
	}
}

func (fsys *FS) ReaderOptions(opts *blob.ReaderOptions) *FS {
	return &FS{
		Bucket:     fsys.Bucket,
		log:        fsys.log,
		readerOpts: opts,
	}
}

func (fsys *FS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	fsys.debugLog(ctx, "openfile", "name", name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	reader, err := fsys.Bucket.NewReader(ctx, name, fsys.readerOpts)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			err = errors.Join(err, fs.ErrNotExist)
		}
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  err,
		}
	}
	return &file{
		ReadCloser: reader,
		info: &fileInfo{
			name:    path.Base(name),
			size:    reader.Size(),
			modTime: reader.ModTime(),
		},
	}, nil
}

func (fsys *FS) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	fsys.debugLog(ctx, "readdir", "name", name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	const pageSize = 1000
	var (
		opts = &blob.ListOptions{
			Delimiter: "/",
		}
		token   = blob.FirstPageToken
		list    []*blob.ListObject
		err     error
		results []fs.DirEntry
	)
	if name != "." {
		opts.Prefix = name + "/"
	}
	for {
		list, token, err = fsys.Bucket.ListPage(ctx, token, pageSize, opts)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if gcerrors.Code(err) == gcerrors.NotFound {
				err = errors.Join(err, fs.ErrNotExist)
			}
			return nil, &fs.PathError{
				Op:   "readdir",
				Path: name,
				Err:  err,
			}
		}
		for _, item := range list {
			inf := &fileInfo{
				name:    path.Base(item.Key),
				size:    item.Size,
				modTime: item.ModTime,
			}
			if item.IsDir {
				inf.mode = fs.ModeDir
			}
			results = append(results, inf)
		}
		if len(token) == 0 {
			break
		}
	}
	// if results are empty, the directory is considered non-existent (an
	// error), except when reading top-level directory
	if len(results) == 0 && name != "." {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}
	return results, nil
}

func (fsys *FS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	fsys.debugLog(ctx, "write", "name", name)
	if !fs.ValidPath(name) || name == "." {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	writer, err := fsys.Bucket.NewWriter(ctx, name, fsys.writerOpts)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	n, writeErr := writer.ReadFrom(r)
	closeErr := writer.Close()
	if writeErr != nil {
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  writeErr,
		}
	}
	if closeErr != nil {
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  closeErr,
		}
	}
	return n, nil
}

func (fsys *FS) Remove(ctx context.Context, name string) error {
	fsys.debugLog(ctx, "remove", "name", name)
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	if err := fsys.Bucket.Delete(ctx, name); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}
	return nil
}

func (fsys *FS) RemoveAll(ctx context.Context, name string) error {
	fsys.debugLog(ctx, "removeall", "name", name)
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	listOpt := &blob.ListOptions{
		Prefix: name + "/",
	}
	list := fsys.Bucket.List(listOpt)
	for {
		next, err := list.Next(ctx)
		if err != nil && !errors.Is(err, io.EOF) {
			return &fs.PathError{
				Op:   "remove",
				Path: name,
				Err:  err,
			}
		}
		if next == nil {
			break
		}
		fsys.debugLog(ctx, "removeall.delete", "name", next.Key)
		if err := fsys.Bucket.Delete(ctx, next.Key); err != nil {
			return &fs.PathError{
				Op:   "remove",
				Path: next.Key,
				Err:  err,
			}
		}
	}
	return nil
}

func (fsys *FS) Copy(ctx context.Context, dst, src string) error {
	fsys.debugLog(ctx, "copy", "dst", dst, "src", src)
	for _, p := range []string{src, dst} {
		if !fs.ValidPath(p) {
			return &fs.PathError{
				Op:   "copy",
				Path: p,
				Err:  fs.ErrInvalid,
			}
		}
		if p == "." {
			return &fs.PathError{
				Op:   "copy",
				Path: p,
				Err:  fs.ErrInvalid,
			}
		}
	}
	return fsys.Bucket.Copy(ctx, dst, src, &blob.CopyOptions{})
}

// ObjectRoots implements ObjectRootIterator
func (fsys *FS) ObjectRoots(ctx context.Context, sel ocfl.PathSelector, fn func(obj *ocfl.ObjectRoot) error) error {
	if fsys.ObjectRootsUseWalkDirs {
		return fsys.objectRootsWalkDirs(ctx, sel, fn)
	}
	return fsys.objectRootsList(ctx, sel, fn)
}

// an ObjectRoots strategy based on WalkDirs
func (fsys *FS) objectRootsWalkDirs(ctx context.Context, sel ocfl.PathSelector, fn func(obj *ocfl.ObjectRoot) error) error {
	fsys.debugLog(ctx, "objectroots", "dir", sel.Path(), "strategy", "walkdir")
	walkFn := func(name string, entries []fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		objRoot := ocfl.NewObjectRoot(fsys, name, entries)
		if objRoot.HasNamaste() {
			if err := fn(objRoot); err != nil {
				return err
			}
			// don't walk object subdirectories
			return walkdirs.ErrSkipDirs
		}
		return nil
	}
	return walkdirs.WalkDirs(ctx, fsys, sel.Path(), sel.SkipDir, walkFn, fsys.ObjectRootWalkDirsGos)
}

// an ObjectRoots strategy based on List()
func (fsys *FS) objectRootsList(ctx context.Context, sel ocfl.PathSelector, fn func(obj *ocfl.ObjectRoot) error) error {
	dir := sel.Path()
	fsys.debugLog(ctx, "objectroots", "dir", dir, "strategy", "listkeys")
	if !fs.ValidPath(dir) {
		return &fs.PathError{
			Op:   "each_object",
			Path: dir,
			Err:  fs.ErrInvalid,
		}
	}
	var opts blob.ListOptions
	if dir != "." {
		opts.Prefix = dir + "/"
	}
	var obj *ocfl.ObjectRoot
	iter := fsys.List(&opts)
	for {
		item, err := iter.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		keyDir := path.Dir(item.Key)
		if sel.SkipDir(keyDir) {
			continue
		}
		keyBase := path.Base(item.Key)
		decl, err := ocfl.ParseNamaste(keyBase)
		if err == nil && decl.Type == ocfl.NamasteTypeObject {
			// new object declaration: apply fn to existing obj and reset
			if obj != nil {
				fsys.debugLog(ctx, "objectroots.complete",
					"dir", obj.Path,
					"alg", obj.SidecarAlg,
					"has_declaration", obj.HasNamaste(),
					"has_inventory", obj.HasInventory(),
					"has_sidecar", obj.HasSidecar(),
					"versions", obj.VersionDirs,
				)
				if err := fn(obj); err != nil {
					return err
				}
			}
			obj = &ocfl.ObjectRoot{
				FS:    fsys,
				Path:  keyDir,
				Spec:  decl.Version,
				Flags: ocfl.HasNamaste,
			}
			continue
		}
		// only continue with this key if is within the object's root
		if obj == nil || !strings.HasPrefix(item.Key, obj.Path) {
			fsys.debugLog(ctx, "objectroots.skipping", "key", item.Key)
			continue
		}
		// item path relative to the object root
		// handle object root file entries
		if keyDir == obj.Path {
			// inventory.json
			if keyBase == "inventory.json" {
				obj.Flags = obj.Flags | ocfl.HasInventory
				continue
			}
			// sidecar
			if strings.HasPrefix(keyBase, "inventory.json.") {
				obj.SidecarAlg = strings.TrimPrefix(keyBase, "inventory.json.")
				obj.Flags |= ocfl.HasSidecar
				continue
			}

		}
		// directories in object root: versions and extensions
		child, _, _ := strings.Cut(strings.TrimPrefix(item.Key, obj.Path+"/"), "/")
		if child == "extensions" && (obj.Flags&ocfl.HasExtensions) == 0 {
			obj.Flags |= ocfl.HasExtensions
			continue
		}
		var v ocfl.VNum
		if err := ocfl.ParseVNum(child, &v); err == nil {
			var found bool
			for _, prev := range obj.VersionDirs {
				if v == prev {
					found = true
					break
				}
			}
			if !found {
				obj.VersionDirs = append(obj.VersionDirs, v)
			}
			continue
		}
		// otherwise, non-conforming file
		obj.NonConform = append(obj.NonConform, child)
		continue
	}
	// haven't called fn on final object
	if obj != nil {
		return fn(obj)
	}
	return nil
}

func (fsys *FS) debugLog(ctx context.Context, method string, args ...any) {
	if fsys.log == nil {
		return
	}
	fsys.log.DebugContext(ctx, method, args...)
}
