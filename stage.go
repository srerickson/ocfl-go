package ocfl

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// FileStager records the file that staged content with a digest was read
// from. It is used by [StageFS] and [StageFile], and implemented by
// [*FileContentMap].
type FileStager interface {
	StageFile(dig string, fsys ocflfs.FS, name string)
}

// BytesStager records staged content with a digest held in memory. It is
// used by [StageBytes], and implemented by [*BytesContentMap].
type BytesStager interface {
	StageBytes(dig string, b []byte)
}

// StageFS adds all files in the directory dir in fsys to u, in the directory
// dst ("." for the top-level directory), and records the files with content
// that u needs (see [ObjectUpdate.NeedsContent]) in content. By default,
// hidden files and directories (with names starting with ".") are skipped;
// use [StageWithHidden] or [StageWithFilter] to change which files are added.
// Files are digested with u's digest algorithm and any fixity algorithms
// from [StageWithFixity], using the goroutines set by [StageWithGoLimit].
// Nothing is added to u or content if there is an error. u and content
// must not be nil.
func StageFS(ctx context.Context, u *ObjectUpdate, content FileStager, fsys ocflfs.FS, dir, dst string, opts ...StageOption) error {
	if u == nil || content == nil {
		panic("ocfl: StageFS: u and content must not be nil")
	}
	if !fs.ValidPath(dst) {
		return &MapPathInvalidErr{Path: dst}
	}
	o := newStageOptions(opts...)
	var files []*ocflfs.FileRef
	for ref, err := range ocflfs.WalkFiles(ctx, fsys, dir) {
		if err != nil {
			return fmt.Errorf("staging %q: %w", dir, err)
		}
		if o.filter == nil || o.filter(ref) {
			files = append(files, ref)
		}
	}
	if err := stageFiles(ctx, u, content, files, o, func(ref *ocflfs.FileRef) string {
		return path.Join(dst, ref.Path)
	}); err != nil {
		return fmt.Errorf("staging %q: %w", dir, err)
	}
	return nil
}

// StageFile adds the file name in fsys to u as dst and, if u needs its
// content, records the file in content. The file is digested with u's digest
// algorithm and any fixity algorithms from [StageWithFixity]. The file is
// added even if it is hidden: [StageWithHidden] and [StageWithFilter] only
// apply to [StageFS]. u and content must not be nil.
func StageFile(ctx context.Context, u *ObjectUpdate, content FileStager, fsys ocflfs.FS, name, dst string, opts ...StageOption) error {
	if u == nil || content == nil {
		panic("ocfl: StageFile: u and content must not be nil")
	}
	files := []*ocflfs.FileRef{{FS: fsys, Path: name}}
	if err := stageFiles(ctx, u, content, files, newStageOptions(opts...), func(*ocflfs.FileRef) string {
		return dst
	}); err != nil {
		return fmt.Errorf("staging %q: %w", name, err)
	}
	return nil
}

// StageBytes adds a file with the content b to u as dst and, if u needs the
// content, records it in content. b is digested with u's digest algorithm and
// the fixity algorithms. u and content must not be nil.
func StageBytes(u *ObjectUpdate, content BytesStager, dst string, b []byte, fixity ...digest.Algorithm) error {
	if u == nil || content == nil {
		panic("ocfl: StageBytes: u and content must not be nil")
	}
	alg := u.DigestAlgorithm()
	digester := digest.NewMultiDigester(append([]digest.Algorithm{alg}, fixity...)...)
	if _, err := digester.Write(b); err != nil {
		return err
	}
	sums := digester.Sums()
	dig := sums[alg.ID()]
	delete(sums, alg.ID())
	if err := u.Add(dst, dig, sums); err != nil {
		return err
	}
	if u.NeedsContent(dig) {
		content.StageBytes(dig, b)
	}
	return nil
}

// StageOption is an optional argument for [StageFS] and [StageFile]. Each
// option documents the functions that use it; others ignore it.
type StageOption func(*stageOptions)

type stageOptions struct {
	fixity  []digest.Algorithm
	filter  func(*ocflfs.FileRef) bool
	goLimit int
}

func newStageOptions(opts ...StageOption) *stageOptions {
	o := &stageOptions{filter: ocflfs.IsNotHidden}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// StageWithFixity sets fixity algorithms to digest added files with, in
// addition to the update's digest algorithm. It is used by [StageFS] and
// [StageFile].
func StageWithFixity(algs ...digest.Algorithm) StageOption {
	return func(o *stageOptions) {
		o.fixity = algs
	}
}

// StageWithHidden includes hidden files and directories (with names starting
// with ".") in [StageFS], which skips them by default. It replaces any filter
// set by [StageWithFilter].
func StageWithHidden() StageOption {
	return func(o *stageOptions) {
		o.filter = nil
	}
}

// StageWithFilter sets a function that decides which files [StageFS] adds: a
// file is added if keep returns true for it. The *FileRef's Path is relative
// to the directory being added. keep replaces the default filter, which skips
// hidden files and directories, and any set by [StageWithHidden].
func StageWithFilter(keep func(*ocflfs.FileRef) bool) StageOption {
	return func(o *stageOptions) {
		o.filter = keep
	}
}

// StageWithGoLimit sets the number of goroutines used by [StageFS] and
// [StageFile] to digest files concurrently. Without it, or if gos is less
// than 1, runtime.GOMAXPROCS(0) is used.
func StageWithGoLimit(gos int) StageOption {
	return func(o *stageOptions) {
		o.goLimit = gos
	}
}

// stageFiles digests files and adds them to u with names from dstName, using
// the fixity algorithms and goroutines from o, and records the files with
// content u needs in content. All files are added, or none of them. files is
// a slice rather than an iterator so that listing errors are found before
// any goroutines start digesting.
func stageFiles(ctx context.Context, u *ObjectUpdate, content FileStager, files []*ocflfs.FileRef, o *stageOptions, dstName func(*ocflfs.FileRef) string) error {
	for _, err := range ocflfs.CheckFileTypes(ctx, slices.Values(files)) {
		if err != nil {
			return err
		}
	}
	alg := u.DigestAlgorithm()
	digested := make([]*digest.FileRef, 0, len(files))
	for ref, err := range digest.DigestFilesBatch(ctx, slices.Values(files), o.goLimit, alg, o.fixity...) {
		if err != nil {
			return err
		}
		digested = append(digested, ref)
	}
	// digests from concurrent goroutines arrive in any order: sort them so
	// the content location for duplicate content doesn't depend on timing.
	slices.SortFunc(digested, func(a, b *digest.FileRef) int {
		return strings.Compare(a.FullPath(), b.FullPath())
	})
	entries := make([]UpdateEntry, len(digested))
	for i, ref := range digested {
		entries[i] = UpdateEntry{
			Name:   dstName(&ref.FileRef),
			Digest: ref.Digests[alg.ID()],
			Fixity: ref.Fixity,
		}
	}
	if err := u.AddAll(entries); err != nil {
		return err
	}
	for i, ref := range digested {
		if dig := entries[i].Digest; u.NeedsContent(dig) {
			content.StageFile(dig, ref.FS, ref.FullPath())
		}
	}
	return nil
}
