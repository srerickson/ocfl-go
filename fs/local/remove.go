package local

import (
	"context"
	"errors"
	"io/fs"
	"os"
)

// Remove removes the file with path name. It satisfies the WriteFS.Remove
// contract: a missing file yields an error that satisfies
// errors.Is(err, fs.ErrNotExist) (the underlying os.Remove error), while
// removing the top-level directory (".") is rejected without touching the
// storage root, though with a backend-specific error (not fs.ErrNotExist).
func (fsys *FS) Remove(ctx context.Context, name string) error {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	if err := ctx.Err(); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if err := os.Remove(fullPath); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	return nil
}

func (fsys *FS) RemoveAll(ctx context.Context, name string) error {
	if _, err := fsys.osPath(name); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	if err := ctx.Err(); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	// Removal is driven through an os.Root opened on the storage root
	// rather than os.RemoveAll(fullPath): os.RemoveAll opens the parent
	// directory by full path (OpenFile(parentDir)), so a symlink at an
	// INTERMEDIATE path component — e.g. root/link -> /external — is
	// followed, and RemoveAll deletes data outside the storage root
	// while reporting nil. os.Root-based removal walks every component
	// with openat family operations (O_NOFOLLOW plus symlink-target
	// validation), so a name can never resolve outside the root: an
	// intermediate symlink whose target escapes the root (absolute, or a
	// relative target resolving out) makes RemoveAll fail with an error
	// instead of deleting; a relative target staying inside the root is
	// followed like any other directory. The final component is unlinked
	// without being followed, so a symlink at the name itself is still
	// removed as a link and its referent — even one outside the root —
	// is never touched. A missing name is a no-op (nil), matching
	// os.RemoveAll semantics.
	root, err := os.OpenRoot(fsys.path)
	if err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	defer root.Close()
	if err := root.RemoveAll(name); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	return nil
}
