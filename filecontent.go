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

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// FileContentMap maps digests to files in FSs that hold content with the
// digest. It implements [ContentSource], [ContentFastChecker] and
// [FileAdder]. The zero value is an empty FileContentMap, ready to use. For
// content in memory, use a [BytesContentMap].
//
// A FileContentMap can be saved as JSON if each file's FS implements
// [encoding.TextMarshaler], as the storage backends in this module do. Each
// FS is saved as the text from its MarshalText method, and file paths are
// saved relative to their FS.
//
// A FileContentMap loaded from JSON keeps each FS as its saved text: decoding
// does no I/O, so a FileContentMap whose content is no longer available can
// still be loaded, changed and saved. Call [FileContentMap.OpenFS] to open the
// FSs before using the FileContentMap as a [ContentSource]: until then,
// GetContent finds no content in them.
//
// A file's size and content token (see [ocflfs.ContentToken]) can be recorded
// when it is added, and are saved with it. [FileContentMap.FastCheck] uses
// them to find files that have changed since they were added, so that a changed file
// isn't copied into an object under the digest of its old content. The check
// is only as good as the storage backend's content tokens: a file changed
// without changing its size may go undetected if its FS provides no tokens,
// or if it changed within the token's time resolution (see the backend's
// documentation).
type FileContentMap struct {
	sources []contentMapSource
	files   map[string]contentMapFile // digest -> file
}

// AddFile sets the location of content with the digest dig to the file name
// in fsys. info is the file's information, from which its size and content
// token are recorded for [FileContentMap.FastCheck]. info should be read before the
// file's content is digested: then a change made while the file is being
// digested is found by FastCheck. info may be nil, or have a negative size or
// no content token, if they are unknown: FastCheck doesn't compare what is
// unknown, and if both are, it only checks that the file exists. fsys must
// not be nil.
func (c *FileContentMap) AddFile(dig string, fsys ocflfs.FS, name string, info fs.FileInfo) {
	dig = normalizeDigest(dig)
	file := contentMapFile{name: name, size: -1}
	if info != nil {
		file.size = max(info.Size(), -1)
		file.token = ocflfs.ContentToken(info)
	}
	src := slices.IndexFunc(c.sources, func(s contentMapSource) bool { return sameFS(s.fs, fsys) })
	if src < 0 {
		src = len(c.sources)
		c.sources = append(c.sources, contentMapSource{fs: fsys})
	}
	if c.files == nil {
		c.files = map[string]contentMapFile{}
	}
	file.src = src
	c.files[dig] = file
}

// Remove removes the content location for the digest dig.
func (c *FileContentMap) Remove(dig string) {
	delete(c.files, normalizeDigest(dig))
}

// DeleteFunc removes the content location for each digest for which del
// returns true. Use it with [ObjectUpdate.NeedsContent] to remove content
// that an update no longer needs.
func (c *FileContentMap) DeleteFunc(del func(dig string) bool) {
	maps.DeleteFunc(c.files, func(dig string, _ contentMapFile) bool { return del(dig) })
}

// Digests returns the sorted digests for all content in c.
func (c *FileContentMap) Digests() []string {
	return slices.Sorted(maps.Keys(c.files))
}

// GetContent implements [ContentSource] for *FileContentMap. Content in an FS
// that was loaded from JSON and hasn't been opened with [FileContentMap.OpenFS] is
// not found.
func (c *FileContentMap) GetContent(dig string) (ocflfs.FS, string) {
	dig = normalizeDigest(dig)
	if file, ok := c.files[dig]; ok {
		if fsys := c.sources[file.src].fs; fsys != nil {
			return fsys, file.name
		}
		return nil, ""
	}
	return nil, ""
}

// OpenFS opens the FSs loaded from JSON that c's content is in, using reg to
// open each FS from its saved text. It must be called before c is used as a
// [ContentSource] (for example, by [ObjectUpdate.Apply]). FSs that are already
// open and FSs that no content is in are skipped, so calling OpenFS again is
// safe. If any FS can't be opened, OpenFS returns all of the errors joined, each
// naming the FS's saved text, and keeps the FSs that did open.
func (c *FileContentMap) OpenFS(ctx context.Context, reg ocflfs.Registry) error {
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

// FastCheck is a fast check that content hasn't changed since it was added to
// c. It does not read or digest any file's content, so it does not validate
// that content still matches its digest: it only compares the file's current
// size and content token with the ones recorded when it was added.
//
// FastCheck stats every file that c's content is in, and returns a
// *[ContentChangedError], which wraps [ErrContentChanged], listing files that
// are missing, or whose size or content token differs from the one recorded
// when they were added. A file's size isn't compared if it wasn't recorded or
// its FS reports a negative size (for example, an HTTP server that doesn't
// send the content length). Likewise, its content token isn't compared if it
// wasn't recorded or its FS provides none. Content in an FS that isn't open
// is reported as missing, so c must be opened with [FileContentMap.OpenFS] first.
// If a file can't be checked for another reason, the error is returned,
// joined with any ContentChangedError.
func (c *FileContentMap) FastCheck(ctx context.Context) error {
	return c.ContentFastCheck(ctx, slices.Sorted(maps.Keys(c.files)))
}

// ContentFastCheck implements [ContentFastChecker] for *FileContentMap. It is
// [FileContentMap.FastCheck] for the content with the given digests only: like
// FastCheck, it does not validate the content's digests. Digests that c has no
// file for are skipped.
func (c *FileContentMap) ContentFastCheck(ctx context.Context, digests []string) error {
	var changes []ContentChange
	var errs []error
	for _, dig := range digests {
		if err := ctx.Err(); err != nil {
			return err
		}
		dig = normalizeDigest(dig)
		file, ok := c.files[dig]
		if !ok {
			continue
		}
		change := ContentChange{
			Digest: dig,
			FS:     c.sources[file.src].fs,
			Path:   file.name,
			Size:   file.size,
			Token:  file.token,
		}
		if change.FS == nil {
			change.Missing = true
			changes = append(changes, change)
			continue
		}
		info, err := ocflfs.StatFile(ctx, change.FS, file.name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			change.Missing = true
		case err != nil:
			errs = append(errs, fmt.Errorf("checking content %q: %w", dig, err))
			continue
		case info.IsDir():
			change.Missing = true
		default:
			change.NewSize = info.Size()
			change.NewToken = ocflfs.ContentToken(info)
			sizeChanged := file.size >= 0 && change.NewSize >= 0 && change.NewSize != file.size
			tokenChanged := file.token != "" && change.NewToken != "" && change.NewToken != file.token
			if !sizeChanged && !tokenChanged {
				continue
			}
		}
		changes = append(changes, change)
	}
	if len(changes) > 0 {
		errs = append([]error{&ContentChangedError{Changes: changes}}, errs...)
	}
	return errors.Join(errs...)
}

// MarshalJSON implements [json.Marshaler] for FileContentMap. It returns an
// error if any FS can't be marshaled as text. An FS
// loaded from JSON is saved as the same text, whether or not it was opened.
func (c FileContentMap) MarshalJSON() ([]byte, error) {
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
		file.src = idx
		j.Content[dig] = file
	}
	return json.Marshal(j)
}

// UnmarshalJSON implements [json.Unmarshaler] for *FileContentMap. It replaces c
// with the FileContentMap saved in data. It does no I/O: each FS is kept as its
// saved text until [FileContentMap.OpenFS] is called.
func (c *FileContentMap) UnmarshalJSON(data []byte) error {
	var j contentMapJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return fmt.Errorf("decoding content map: %w", err)
	}
	loaded := FileContentMap{
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

// contentMapSource is an FS in a FileContentMap. A source loaded from JSON has the
// saved text, and fs is nil until it is opened. A source added with
// [FileContentMap.AddFile] has fs and no text.
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

// contentMapFile is the location of a file in a FileContentMap
type contentMapFile struct {
	src   int    // index of the FS in the content map's sources
	name  string // path relative to the FS
	size  int64  // size of the file when it was added; -1 if unknown
	token string // content token when the file was added; "" if unknown
}

// MarshalJSON encodes f as an array: [src, name, size, token], with null for
// an unknown size or token.
func (f contentMapFile) MarshalJSON() ([]byte, error) {
	var size *int64
	if f.size >= 0 {
		size = &f.size
	}
	var token *string
	if f.token != "" {
		token = &f.token
	}
	return json.Marshal([]any{f.src, f.name, size, token})
}

// UnmarshalJSON decodes f from an array: [src, name, size, token], where size
// and token may be null.
func (f *contentMapFile) UnmarshalJSON(data []byte) error {
	var parts []json.RawMessage
	if err := json.Unmarshal(data, &parts); err != nil {
		return err
	}
	if len(parts) != 4 {
		return fmt.Errorf("content map entry must have four elements (source, path, size, token), not %d", len(parts))
	}
	if err := json.Unmarshal(parts[0], &f.src); err != nil {
		return fmt.Errorf("content map entry source index: %w", err)
	}
	if err := json.Unmarshal(parts[1], &f.name); err != nil {
		return fmt.Errorf("content map entry path: %w", err)
	}
	var size *int64
	if err := json.Unmarshal(parts[2], &size); err != nil {
		return fmt.Errorf("content map entry size: %w", err)
	}
	f.size = -1
	if size != nil {
		if *size < 0 {
			return fmt.Errorf("content map entry has a negative size: %d", *size)
		}
		f.size = *size
	}
	var token *string
	if err := json.Unmarshal(parts[3], &token); err != nil {
		return fmt.Errorf("content map entry token: %w", err)
	}
	f.token = ""
	if token != nil {
		if *token == "" {
			return errors.New("content map entry has an empty token: an unknown token must be null")
		}
		f.token = *token
	}
	return nil
}

// contentMapJSON is the saved form of a FileContentMap
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

// sameFS returns true if a and b are the same FS value. Values that can't be
// compared, including values of comparable types that hold uncomparable
// values (such as a struct with an interface field holding a map), are never
// the same.
func sameFS(a, b ocflfs.FS) bool {
	if reflect.TypeOf(a) != reflect.TypeOf(b) || !reflect.ValueOf(a).Comparable() {
		return false
	}
	return a == b
}
