package ocfl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// Stage is a draft [ObjectUpdate] paired with a [ContentMap] holding the
// locations of new content. The stage keeps them consistent: content added to
// the stage is recorded in both, and content that is no longer needed is
// dropped from both. Create a stage with [NewStage], [Root.NewStage] or
// [Object.NewStage], edit it, then call [Stage.Finalize] and apply its
// [Stage.Update] using its [Stage.Content] as the [ContentSource].
//
// A Stage can be saved as JSON (see [ContentMap] for restrictions) and loaded
// with [json.Unmarshal], which does no I/O. Call [ContentMap.OpenFS] on a loaded
// stage's Content before applying its Update.
//
// Files added to a stage must not change before the stage is applied. The
// size and content token of each file are recorded when it is added: use
// [ContentMap.FastCheck] to find files that are missing or have changed.
// [ObjectUpdate.Apply] runs the same check before writing anything.
type Stage struct {
	update  *ObjectUpdate
	content *ContentMap
}

// NewStage returns a new *Stage for the object at dir in fsys. The stage's
// update is created as with [NewUpdate], which documents id, and its
// ContentMap is empty. NewStage uses the option [StageWithDigestAlgorithm].
func NewStage(ctx context.Context, fsys ocflfs.FS, dir, id string, opts ...StageOption) (*Stage, error) {
	u, err := NewUpdate(ctx, fsys, dir, id, newStageOptions(opts...).updateOptions()...)
	if err != nil {
		return nil, err
	}
	return newStage(u), nil
}

// newStage returns a stage for the draft u, which must not have new content.
func newStage(u *ObjectUpdate) *Stage {
	return &Stage{update: u, content: &ContentMap{}}
}

// Update returns the stage's update. Use it to inspect the update and, after
// [Stage.Finalize], to apply or revert it. Edit the update through the stage:
// the update's own edit methods don't change the stage's ContentMap.
func (s *Stage) Update() *ObjectUpdate { return s.update }

// Content returns the stage's ContentMap, the [ContentSource] for applying
// the stage's update.
func (s *Stage) Content() *ContentMap { return s.content }

// Finalize finalizes the stage's update, after which the stage can't be
// edited. See [ObjectUpdate.Finalize].
func (s *Stage) Finalize(msg string, user User, opts ...UpdateOption) error {
	return s.update.Finalize(msg, user, opts...)
}

// AddFS adds all files in the directory dir in fsys to the stage, in the
// directory dst ("." for the top-level directory). By default, hidden files
// and directories (with names starting with ".") are skipped; use
// [StageWithHidden] or [StageWithFilter] to change which files are added.
// Files are digested with the update's digest algorithm and any fixity
// algorithms from [StageWithFixity], using the goroutines set by
// [StageWithGoLimit]. Nothing is added if there is an error.
func (s *Stage) AddFS(ctx context.Context, fsys ocflfs.FS, dir, dst string, opts ...StageOption) error {
	if !fs.ValidPath(dst) {
		return &MapPathInvalidErr{Path: dst}
	}
	o := newStageOptions(opts...)
	var files []*ocflfs.FileRef
	for ref, err := range ocflfs.WalkFiles(ctx, fsys, dir) {
		if err != nil {
			return fmt.Errorf("adding %q to stage: %w", dir, err)
		}
		if o.filter == nil || o.filter(ref) {
			files = append(files, ref)
		}
	}
	if err := s.addFiles(ctx, files, o, func(ref *ocflfs.FileRef) string {
		return path.Join(dst, ref.Path)
	}); err != nil {
		return fmt.Errorf("adding %q to stage: %w", dir, err)
	}
	return nil
}

// AddFile adds the file name in fsys to the stage as dst. The file is
// digested with the update's digest algorithm and any fixity algorithms from
// [StageWithFixity]. The file is added even if it is hidden: [StageWithHidden]
// and [StageWithFilter] only apply to [Stage.AddFS].
func (s *Stage) AddFile(ctx context.Context, fsys ocflfs.FS, name, dst string, opts ...StageOption) error {
	files := []*ocflfs.FileRef{{FS: fsys, Path: name}}
	if err := s.addFiles(ctx, files, newStageOptions(opts...), func(*ocflfs.FileRef) string {
		return dst
	}); err != nil {
		return fmt.Errorf("adding %q to stage: %w", name, err)
	}
	return nil
}

// AddBytes adds a file with the content b to the stage as dst. The content is
// digested with the update's digest algorithm and the fixity algorithms. A
// stage with content added by AddBytes can't be saved as JSON.
func (s *Stage) AddBytes(dst string, b []byte, fixity ...digest.Algorithm) error {
	alg := s.update.DigestAlgorithm()
	digester := digest.NewMultiDigester(append([]digest.Algorithm{alg}, fixity...)...)
	if _, err := digester.Write(b); err != nil {
		return err
	}
	sums := digester.Sums()
	dig := sums[alg.ID()]
	delete(sums, alg.ID())
	if err := s.update.Add(dst, dig, sums); err != nil {
		return err
	}
	if s.update.needsContent(dig) {
		s.content.AddBytes(dig, b)
	}
	s.pruneContent()
	return nil
}

// Remove removes the file or directory name from the stage. Content that is
// no longer needed is removed from the stage's ContentMap.
func (s *Stage) Remove(name string) error {
	if err := s.update.Remove(name); err != nil {
		return err
	}
	s.pruneContent()
	return nil
}

// Rename renames the file or directory src in the stage to dst.
func (s *Stage) Rename(src, dst string) error {
	return s.update.Rename(src, dst)
}

// Clear removes all files from the stage, leaving its update with an empty
// state and its ContentMap with no content.
func (s *Stage) Clear() error {
	if err := s.update.Clear(); err != nil {
		return err
	}
	s.pruneContent()
	return nil
}

// stageJSON is the JSON form of a Stage.
type stageJSON struct {
	Update  *ObjectUpdate `json:"update"`
	Content *ContentMap   `json:"content"`
}

// MarshalJSON implements [json.Marshaler] for Stage.
func (s Stage) MarshalJSON() ([]byte, error) {
	return json.Marshal(stageJSON{Update: s.update, Content: s.content})
}

// UnmarshalJSON implements [json.Unmarshaler] for *Stage. It returns an
// error if data has fields other than "update" and "content", or is missing
// either of them.
func (s *Stage) UnmarshalJSON(data []byte) error {
	var loaded stageJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&loaded); err != nil {
		return fmt.Errorf("decoding stage: %w", err)
	}
	if loaded.Update == nil || loaded.Content == nil {
		return errors.New("decoding stage: missing 'update' or 'content'")
	}
	*s = Stage{update: loaded.Update, content: loaded.Content}
	return nil
}

// addFiles digests files and adds them to the stage with names from
// dstName, using the fixity algorithms and goroutines from o. All files are
// added, or none of them. files is a slice rather than an iterator so that
// listing errors are found before any goroutines start digesting.
func (s *Stage) addFiles(ctx context.Context, files []*ocflfs.FileRef, o *stageOptions, dstName func(*ocflfs.FileRef) string) error {
	for _, err := range ocflfs.CheckFileTypes(ctx, slices.Values(files)) {
		if err != nil {
			return err
		}
	}
	alg := s.update.DigestAlgorithm()
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
	entries := make([]updateEntry, len(digested))
	for i, ref := range digested {
		entries[i] = updateEntry{
			name:   dstName(&ref.FileRef),
			digest: ref.Digests[alg.ID()],
			fixity: ref.Fixity,
		}
	}
	if err := s.update.addAll(entries); err != nil {
		return err
	}
	for i, ref := range digested {
		// Info was set before the file was digested, by the walk or by
		// CheckFileTypes, so if the file changed while it was being
		// digested, its recorded content token is older than the digested
		// content and ContentMap.FastCheck reports the change.
		if dig := entries[i].digest; s.update.needsContent(dig) {
			s.content.AddFile(dig, ref.FS, ref.FullPath(), ref.Info)
		}
	}
	s.pruneContent()
	return nil
}

// pruneContent removes content that the update doesn't need from the stage's
// ContentMap.
func (s *Stage) pruneContent() {
	for _, dig := range s.content.Digests() {
		if !s.update.needsContent(dig) {
			s.content.Remove(dig)
		}
	}
}

// StageOption is an optional argument for creating a [Stage] and for
// [Stage.AddFS] and [Stage.AddFile]. Each function documents the functions
// and methods that use it; others ignore it.
type StageOption func(*stageOptions)

type stageOptions struct {
	alg     digest.Algorithm
	fixity  []digest.Algorithm
	filter  func(*ocflfs.FileRef) bool
	goLimit int
}

func newStageOptions(opts ...StageOption) *stageOptions {
	o := &stageOptions{filter: ocflfs.IsNotHidden, goLimit: 1}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// updateOptions returns the options for creating the stage's update.
func (o *stageOptions) updateOptions() []UpdateOption {
	if o.alg == nil {
		return nil
	}
	return []UpdateOption{UpdateWithDigestAlgorithm(o.alg)}
}

// StageWithDigestAlgorithm sets the primary digest algorithm (sha512 or
// sha256) for a new object, as with [UpdateWithDigestAlgorithm]. It is used by
// [NewStage] and [Root.NewStage], and ignored if the object exists.
func StageWithDigestAlgorithm(alg digest.Algorithm) StageOption {
	return func(o *stageOptions) {
		o.alg = alg
	}
}

// StageWithFixity sets fixity algorithms to digest added files with, in
// addition to the update's digest algorithm. It is used by [Stage.AddFS] and
// [Stage.AddFile].
func StageWithFixity(algs ...digest.Algorithm) StageOption {
	return func(o *stageOptions) {
		o.fixity = algs
	}
}

// StageWithHidden includes hidden files and directories (with names starting
// with ".") in [Stage.AddFS], which skips them by default. It replaces any
// filter set by [StageWithFilter].
func StageWithHidden() StageOption {
	return func(o *stageOptions) {
		o.filter = nil
	}
}

// StageWithFilter sets a function that decides which files [Stage.AddFS]
// adds: a file is added if keep returns true for it. The *FileRef's Path is
// relative to the directory being added. keep replaces the default filter,
// which skips hidden files and directories, and any set by [StageWithHidden].
func StageWithFilter(keep func(*ocflfs.FileRef) bool) StageOption {
	return func(o *stageOptions) {
		o.filter = keep
	}
}

// StageWithGoLimit sets the number of goroutines used by [Stage.AddFS] and
// [Stage.AddFile] to digest files concurrently. The default is 1. If gos is
// less than 1, runtime.GOMAXPROCS(0) is used.
func StageWithGoLimit(gos int) StageOption {
	return func(o *stageOptions) {
		o.goLimit = gos
	}
}
