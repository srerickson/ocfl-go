package ocfl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"path"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// Stage pairs an [ObjectUpdate] with a [ContentMap] holding the locations of
// new content, and keeps them consistent: content added to the stage is
// recorded in both, and content that is no longer needed is dropped from both.
//
// A Stage can be saved as JSON (see [ContentMap] for restrictions) and loaded
// with [json.Unmarshal], which does no I/O. Call [ContentMap.Open] on a loaded
// stage's Content before applying its Update.
type Stage struct {
	Update  *ObjectUpdate `json:"update"`
	Content *ContentMap   `json:"content"`
}

// NewStage returns a new *Stage for the update u, with an empty ContentMap. u
// must not be nil.
func NewStage(u *ObjectUpdate) *Stage {
	return &Stage{Update: u, Content: &ContentMap{}}
}

// AddFS adds all files in the directory dir in fsys to the stage, in the
// directory dst ("." for the top-level directory). Hidden files (with names
// starting with ".") are ignored. Files are digested with the update's digest
// algorithm and the fixity algorithms. Nothing is added if there is an error.
func (s *Stage) AddFS(ctx context.Context, fsys ocflfs.FS, dir, dst string, fixity ...digest.Algorithm) error {
	if !fs.ValidPath(dst) {
		return &MapPathInvalidErr{Path: dst}
	}
	files, walkErr := ocflfs.UntilErr(ocflfs.WalkFiles(ctx, fsys, dir))
	files = ocflfs.FilterFiles(files, ocflfs.IsNotHidden)
	if err := s.addFiles(ctx, files, walkErr, func(ref *ocflfs.FileRef) string {
		return path.Join(dst, ref.Path)
	}, fixity); err != nil {
		return fmt.Errorf("adding %q to stage: %w", dir, err)
	}
	return nil
}

// AddFile adds the file name in fsys to the stage as dst. The file is
// digested with the update's digest algorithm and the fixity algorithms.
func (s *Stage) AddFile(ctx context.Context, fsys ocflfs.FS, name, dst string, fixity ...digest.Algorithm) error {
	files := ocflfs.Files(fsys, name)
	if err := s.addFiles(ctx, files, func() error { return nil }, func(*ocflfs.FileRef) string {
		return dst
	}, fixity); err != nil {
		return fmt.Errorf("adding %q to stage: %w", name, err)
	}
	return nil
}

// AddBytes adds a file with the content b to the stage as dst. The content is
// digested with the update's digest algorithm and the fixity algorithms. A
// stage with content added by AddBytes can't be saved as JSON.
func (s *Stage) AddBytes(dst string, b []byte, fixity ...digest.Algorithm) error {
	alg := s.Update.DigestAlgorithm()
	digester := digest.NewMultiDigester(append([]digest.Algorithm{alg}, fixity...)...)
	if _, err := digester.Write(b); err != nil {
		return err
	}
	sums := digester.Sums()
	dig := sums[alg.ID()]
	delete(sums, alg.ID())
	if err := s.Update.Add(dst, dig, sums); err != nil {
		return err
	}
	if s.Update.needsContent(dig) {
		s.Content.AddBytes(dig, b)
	}
	s.pruneContent()
	return nil
}

// Remove removes the file or directory name from the stage. Content that is
// no longer needed is removed from the stage's ContentMap.
func (s *Stage) Remove(name string) error {
	if err := s.Update.Remove(name); err != nil {
		return err
	}
	s.pruneContent()
	return nil
}

// Rename renames the file or directory src in the stage to dst.
func (s *Stage) Rename(src, dst string) error {
	return s.Update.Rename(src, dst)
}

// UnmarshalJSON implements [json.Unmarshaler] for *Stage. It returns an
// error if data has fields other than "update" and "content", or is missing
// either of them.
func (s *Stage) UnmarshalJSON(data []byte) error {
	type stageFields Stage // without the UnmarshalJSON method
	var loaded stageFields
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&loaded); err != nil {
		return fmt.Errorf("decoding stage: %w", err)
	}
	if loaded.Update == nil || loaded.Content == nil {
		return errors.New("decoding stage: missing 'update' or 'content'")
	}
	*s = Stage(loaded)
	return nil
}

// addFiles digests files and adds them to the stage with names from
// dstName. All files are added, or none of them.
func (s *Stage) addFiles(ctx context.Context, files iter.Seq[*ocflfs.FileRef], filesErr func() error, dstName func(*ocflfs.FileRef) string, fixity []digest.Algorithm) error {
	alg := s.Update.DigestAlgorithm()
	validFiles, fileTypeErr := ocflfs.UntilErr(ocflfs.CheckFileTypes(ctx, files))
	digests, digestErr := ocflfs.UntilErr(digest.DigestFiles(ctx, validFiles, alg, fixity...))
	var entries []updateEntry
	var refs []*digest.FileRef
	for ref := range digests {
		entries = append(entries, updateEntry{
			name:   dstName(&ref.FileRef),
			digest: ref.Digests[alg.ID()],
			fixity: ref.Fixity,
		})
		refs = append(refs, ref)
	}
	for _, errFn := range []func() error{digestErr, fileTypeErr, filesErr} {
		if err := errFn(); err != nil {
			return err
		}
	}
	if err := s.Update.addAll(entries); err != nil {
		return err
	}
	for i, ref := range refs {
		if dig := entries[i].digest; s.Update.needsContent(dig) {
			s.Content.AddFile(dig, ref.FS, ref.FullPath())
		}
	}
	s.pruneContent()
	return nil
}

// pruneContent removes content that the update doesn't need from the stage's
// ContentMap.
func (s *Stage) pruneContent() {
	for _, dig := range s.Content.Digests() {
		if !s.Update.needsContent(dig) {
			s.Content.Remove(dig)
		}
	}
}
