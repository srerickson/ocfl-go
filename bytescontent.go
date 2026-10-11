package ocfl

import (
	"maps"
	"slices"
	"testing/fstest"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// BytesContentMap maps digests to content held in memory. It implements
// [ContentSource] and [BytesStager]. The zero value is an empty
// BytesContentMap, ready to use. A BytesContentMap can't be saved: use a
// [FileContentMap] for content that must outlive the process.
type BytesContentMap struct {
	bytes map[string][]byte // digest -> content
}

// StageBytes sets the content with the digest dig to b. b must not be
// modified after it is staged.
func (c *BytesContentMap) StageBytes(dig string, b []byte) {
	if c.bytes == nil {
		c.bytes = map[string][]byte{}
	}
	c.bytes[normalizeDigest(dig)] = b
}

// Remove removes the content with the digest dig.
func (c *BytesContentMap) Remove(dig string) {
	delete(c.bytes, normalizeDigest(dig))
}

// DeleteFunc removes the content for each digest for which del returns true.
// Use it with [ObjectUpdate.NeedsContent] to remove content that an update
// no longer needs.
func (c *BytesContentMap) DeleteFunc(del func(dig string) bool) {
	maps.DeleteFunc(c.bytes, func(dig string, _ []byte) bool { return del(dig) })
}

// Digests returns the sorted digests for all content in c.
func (c *BytesContentMap) Digests() []string {
	return slices.Sorted(maps.Keys(c.bytes))
}

// GetContent implements [ContentSource] for *BytesContentMap.
func (c *BytesContentMap) GetContent(dig string) (ocflfs.FS, string) {
	b, ok := c.bytes[normalizeDigest(dig)]
	if !ok {
		return nil, ""
	}
	const name = "content"
	return ocflfs.NewWrapFS(fstest.MapFS{name: &fstest.MapFile{Data: b}}), name
}
