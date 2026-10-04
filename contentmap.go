package ocfl

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"testing/fstest"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// ContentSource is used to access content with a given digest when creating and
// updating objects.
type ContentSource interface {
	// GetContent returns an FS and path to a file in FS for a file with the given digest.
	// If no content is associated with the digest, fsys is nil and path is an empty string.
	GetContent(digest string) (fsys ocflfs.FS, path string)
}

// FSOpener opens an FS from text returned by the FS's MarshalText method (see
// [encoding.TextMarshaler]). The fs/config package provides one for the
// storage backends in this module.
type FSOpener func(ctx context.Context, text string) (ocflfs.FS, error)

// ContentMap maps digests to the location of content with the digest: a file
// in an FS or bytes in memory. It implements [ContentSource]. The zero value is
// an empty ContentMap, ready to use.
//
// A ContentMap can be saved as JSON if all of its content is in files and each
// file's FS implements [encoding.TextMarshaler], as the storage backends in
// this module do. Each FS is saved as the text from its MarshalText method,
// and file paths are saved relative to their FS. Use [UnmarshalContentMap] to
// load a saved ContentMap.
type ContentMap struct {
	sources []ocflfs.FS
	files   map[string]contentMapFile // digest -> file
	bytes   map[string][]byte         // digest -> content
}

// UnmarshalContentMap loads a ContentMap saved with [ContentMap.MarshalJSON],
// using open to open each FS that content is stored in. open is required.
func UnmarshalContentMap(ctx context.Context, data []byte, open FSOpener) (*ContentMap, error) {
	var j contentMapJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return nil, fmt.Errorf("decoding content map: %w", err)
	}
	c := &ContentMap{
		sources: make([]ocflfs.FS, len(j.Sources)),
		files:   make(map[string]contentMapFile, len(j.Content)),
	}
	for i, text := range j.Sources {
		fsys, err := open(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("opening content map source %q: %w", text, err)
		}
		c.sources[i] = fsys
	}
	for dig, file := range j.Content {
		if file.src < 0 || file.src >= len(c.sources) {
			return nil, fmt.Errorf("content map entry for %q has an invalid source index: %d", dig, file.src)
		}
		if !fs.ValidPath(file.name) || file.name == "." {
			return nil, fmt.Errorf("content map entry for %q has an invalid path: %q", dig, file.name)
		}
		c.files[normalizeDigest(dig)] = file
	}
	return c, nil
}

// AddFile sets the location of content with the digest dig to the file name
// in fsys. fsys must not be nil.
func (c *ContentMap) AddFile(dig string, fsys ocflfs.FS, name string) {
	dig = normalizeDigest(dig)
	src := slices.IndexFunc(c.sources, func(s ocflfs.FS) bool { return sameFS(s, fsys) })
	if src < 0 {
		src = len(c.sources)
		c.sources = append(c.sources, fsys)
	}
	if c.files == nil {
		c.files = map[string]contentMapFile{}
	}
	delete(c.bytes, dig)
	c.files[dig] = contentMapFile{src: src, name: name}
}

// AddBytes sets the content with the digest dig to b. A ContentMap with
// content in memory can't be saved as JSON.
func (c *ContentMap) AddBytes(dig string, b []byte) {
	dig = normalizeDigest(dig)
	if c.bytes == nil {
		c.bytes = map[string][]byte{}
	}
	delete(c.files, dig)
	c.bytes[dig] = b
}

// Remove removes the content location for the digest dig.
func (c *ContentMap) Remove(dig string) {
	dig = normalizeDigest(dig)
	delete(c.files, dig)
	delete(c.bytes, dig)
}

// Digests returns the sorted digests for all content in c.
func (c *ContentMap) Digests() []string {
	digests := slices.Collect(maps.Keys(c.files))
	digests = slices.AppendSeq(digests, maps.Keys(c.bytes))
	slices.Sort(digests)
	return digests
}

// GetContent implements [ContentSource] for *ContentMap.
func (c *ContentMap) GetContent(dig string) (ocflfs.FS, string) {
	dig = normalizeDigest(dig)
	if file, ok := c.files[dig]; ok {
		return c.sources[file.src], file.name
	}
	if b, ok := c.bytes[dig]; ok {
		const name = "content"
		return ocflfs.NewWrapFS(fstest.MapFS{name: &fstest.MapFile{Data: b}}), name
	}
	return nil, ""
}

// MarshalJSON implements [json.Marshaler] for ContentMap. It returns an error
// if any content is in memory or if any FS can't be marshaled as text.
func (c ContentMap) MarshalJSON() ([]byte, error) {
	if len(c.bytes) > 0 {
		return nil, errors.New("content map has content in memory, which can't be saved: write it to a file first")
	}
	j := contentMapJSON{
		Sources: []string{},
		Content: make(map[string]contentMapFile, len(c.files)),
	}
	// only save sources that are used, in the order they were added, and
	// save sources that marshal to the same text once.
	srcIndex := map[int]int{} // index in c.sources -> index in j.Sources
	for _, dig := range slices.Sorted(maps.Keys(c.files)) {
		file := c.files[dig]
		idx, ok := srcIndex[file.src]
		if !ok {
			text, err := marshalFS(c.sources[file.src])
			if err != nil {
				return nil, err
			}
			idx = slices.Index(j.Sources, text)
			if idx < 0 {
				idx = len(j.Sources)
				j.Sources = append(j.Sources, text)
			}
			srcIndex[file.src] = idx
		}
		j.Content[dig] = contentMapFile{src: idx, name: file.name}
	}
	return json.Marshal(j)
}

// contentMapFile is the location of a file in a ContentMap
type contentMapFile struct {
	src  int    // index of the FS in the content map's sources
	name string // path relative to the FS
}

// MarshalJSON encodes f as a two-element array: [src, name].
func (f contentMapFile) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{f.src, f.name})
}

// UnmarshalJSON decodes f from a two-element array: [src, name].
func (f *contentMapFile) UnmarshalJSON(data []byte) error {
	var parts []json.RawMessage
	if err := json.Unmarshal(data, &parts); err != nil {
		return err
	}
	if len(parts) != 2 {
		return fmt.Errorf("content map entry must have two elements, not %d", len(parts))
	}
	if err := json.Unmarshal(parts[0], &f.src); err != nil {
		return fmt.Errorf("content map entry source index: %w", err)
	}
	if err := json.Unmarshal(parts[1], &f.name); err != nil {
		return fmt.Errorf("content map entry path: %w", err)
	}
	return nil
}

// contentMapJSON is the saved form of a ContentMap
type contentMapJSON struct {
	Sources []string                  `json:"sources"`
	Content map[string]contentMapFile `json:"content"`
}

func marshalFS(fsys ocflfs.FS) (string, error) {
	marshaler, ok := fsys.(encoding.TextMarshaler)
	if !ok {
		return "", fmt.Errorf("content map source %T can't be saved: it doesn't implement encoding.TextMarshaler", fsys)
	}
	text, err := marshaler.MarshalText()
	if err != nil {
		return "", fmt.Errorf("saving content map source: %w", err)
	}
	return string(text), nil
}

// sameFS returns true if a and b are the same FS value. Values of types that
// can't be compared are never the same.
func sameFS(a, b ocflfs.FS) bool {
	typ := reflect.TypeOf(a)
	if typ != reflect.TypeOf(b) || !typ.Comparable() {
		return false
	}
	return a == b
}
