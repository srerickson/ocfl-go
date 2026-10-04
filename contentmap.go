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
	"net/url"
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

// ContentMap maps digests to the location of content with the digest: a file
// in an FS or bytes in memory. It implements [ContentSource]. The zero value is
// an empty ContentMap, ready to use.
//
// A ContentMap can be saved as JSON if all of its content is in files and each
// file's FS implements [encoding.TextMarshaler], as the storage backends in
// this module do. Each FS is saved as the text from its MarshalText method,
// and file paths are saved relative to their FS.
//
// A ContentMap loaded from JSON keeps each FS as its saved text: decoding
// does no I/O, so a ContentMap whose content is no longer available can still
// be loaded, changed and saved. Call [ContentMap.Open] to open the FSs before
// using the ContentMap as a [ContentSource]: until then, GetContent finds no
// content in them.
type ContentMap struct {
	sources []contentMapSource
	files   map[string]contentMapFile // digest -> file
	bytes   map[string][]byte         // digest -> content
}

// AddFile sets the location of content with the digest dig to the file name
// in fsys. fsys must not be nil.
func (c *ContentMap) AddFile(dig string, fsys ocflfs.FS, name string) {
	dig = normalizeDigest(dig)
	src := slices.IndexFunc(c.sources, func(s contentMapSource) bool { return sameFS(s.fs, fsys) })
	if src < 0 {
		src = len(c.sources)
		c.sources = append(c.sources, contentMapSource{fs: fsys})
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

// GetContent implements [ContentSource] for *ContentMap. Content in an FS
// that was loaded from JSON and hasn't been opened with [ContentMap.Open] is
// not found.
func (c *ContentMap) GetContent(dig string) (ocflfs.FS, string) {
	dig = normalizeDigest(dig)
	if file, ok := c.files[dig]; ok {
		if fsys := c.sources[file.src].fs; fsys != nil {
			return fsys, file.name
		}
		return nil, ""
	}
	if b, ok := c.bytes[dig]; ok {
		const name = "content"
		return ocflfs.NewWrapFS(fstest.MapFS{name: &fstest.MapFile{Data: b}}), name
	}
	return nil, ""
}

// Open opens the FSs loaded from JSON that c's content is in, using reg to
// open each FS from its saved text. It must be called before c is used as a
// [ContentSource] (for example, by [ObjectUpdate.Apply]). FSs that are already
// open and FSs that no content is in are skipped, so calling Open again is
// safe. If any FS can't be opened, Open returns all of the errors joined, each
// naming the FS's saved text, and keeps the FSs that did open.
func (c *ContentMap) Open(ctx context.Context, reg ocflfs.Registry) error {
	used := map[int]bool{}
	for _, file := range c.files {
		used[file.src] = true
	}
	var errs []error
	for i := range c.sources {
		src := &c.sources[i]
		if !used[i] || src.fs != nil {
			continue
		}
		fsys, err := reg.Open(ctx, src.text)
		if err != nil {
			errs = append(errs, fmt.Errorf("opening content source %q: %w", redactURL(src.text), err))
			continue
		}
		src.fs = fsys
	}
	return errors.Join(errs...)
}

// MarshalJSON implements [json.Marshaler] for ContentMap. It returns an error
// if any content is in memory or if any FS can't be marshaled as text. An FS
// loaded from JSON is saved as the same text, whether or not it was opened.
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
			text, err := c.sources[file.src].marshal()
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

// UnmarshalJSON implements [json.Unmarshaler] for *ContentMap. It replaces c
// with the ContentMap saved in data. It does no I/O: each FS is kept as its
// saved text until [ContentMap.Open] is called.
func (c *ContentMap) UnmarshalJSON(data []byte) error {
	var j contentMapJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return fmt.Errorf("decoding content map: %w", err)
	}
	loaded := ContentMap{
		sources: make([]contentMapSource, len(j.Sources)),
		files:   make(map[string]contentMapFile, len(j.Content)),
	}
	for i, text := range j.Sources {
		if text == "" {
			return fmt.Errorf("content map source %d is empty", i)
		}
		loaded.sources[i] = contentMapSource{text: text}
	}
	for dig, file := range j.Content {
		if file.src < 0 || file.src >= len(loaded.sources) {
			return fmt.Errorf("content map entry for %q has an invalid source index: %d", dig, file.src)
		}
		if !fs.ValidPath(file.name) || file.name == "." {
			return fmt.Errorf("content map entry for %q has an invalid path: %q", dig, file.name)
		}
		loaded.files[normalizeDigest(dig)] = file
	}
	*c = loaded
	return nil
}

// contentMapSource is an FS in a ContentMap. A source loaded from JSON has the
// saved text, and fs is nil until it is opened. A source added with
// [ContentMap.AddFile] has fs and no text.
type contentMapSource struct {
	text string
	fs   ocflfs.FS
}

// marshal returns the text that src is saved as.
func (src contentMapSource) marshal() (string, error) {
	if src.text != "" {
		return src.text, nil
	}
	marshaler, ok := src.fs.(encoding.TextMarshaler)
	if !ok {
		return "", fmt.Errorf("content map source %T can't be saved: it doesn't implement encoding.TextMarshaler", src.fs)
	}
	text, err := marshaler.MarshalText()
	if err != nil {
		return "", fmt.Errorf("saving content map source: %w", err)
	}
	return string(text), nil
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

// redactURL returns text with any password replaced by "xxxxx", if text is a
// URL. Otherwise it returns text unchanged.
func redactURL(text string) string {
	u, err := url.Parse(text)
	if err != nil {
		return text
	}
	return u.Redacted()
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
