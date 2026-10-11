package ocfl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// ErrContentChanged is returned when files that content was added from are
// missing or have changed since they were added. See [ContentChangedError].
var ErrContentChanged = errors.New("content has changed or is missing")

// ContentSource is used to access content with a given digest when creating and
// updating objects.
type ContentSource interface {
	// GetContent returns an FS and path to a file in FS for a file with the given digest.
	// If no content is associated with the digest, fsys is nil and path is an empty string.
	GetContent(digest string) (fsys ocflfs.FS, path string)
}

// ContentFastChecker is a [ContentSource] that can cheaply check that its
// content hasn't changed since it was added. The check is "fast" because it
// does not read or digest the content: it compares metadata recorded when the
// content was added (such as size and modification time), so it can miss
// changes that leave the metadata the same. It does not validate content
// digests. [ObjectUpdate.Apply] calls ContentFastCheck with the digests of the
// content it will copy, before writing anything, and returns any error without
// writing anything.
type ContentFastChecker interface {
	ContentSource
	ContentFastCheck(ctx context.Context, digests []string) error
}

// MultiContentSource is a [ContentSource] that gets content from the first
// of its sources that has it. It implements [ContentFastChecker]: each digest
// is checked by the source that provides it, if that source is a
// ContentFastChecker.
type MultiContentSource []ContentSource

// GetContent implements [ContentSource] for MultiContentSource.
func (m MultiContentSource) GetContent(dig string) (ocflfs.FS, string) {
	for _, src := range m {
		if fsys, name := getContent(src, dig); fsys != nil {
			return fsys, name
		}
	}
	return nil, ""
}

// ContentFastCheck implements [ContentFastChecker] for MultiContentSource.
// Digests that no source provides are skipped.
func (m MultiContentSource) ContentFastCheck(ctx context.Context, digests []string) error {
	bySource := make([][]string, len(m))
	for _, dig := range digests {
		for i, src := range m {
			if fsys, _ := getContent(src, dig); fsys != nil {
				bySource[i] = append(bySource[i], dig)
				break
			}
		}
	}
	var errs []error
	for i, src := range m {
		checker, ok := src.(ContentFastChecker)
		if !ok || len(bySource[i]) == 0 {
			continue
		}
		if err := checker.ContentFastCheck(ctx, bySource[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ContentChangedError is returned by [FileContentMap.FastCheck] and
// [FileContentMap.ContentFastCheck] (and so by [ObjectUpdate.Apply]) when
// files that content was added from are missing or their size or content
// token has changed. It wraps [ErrContentChanged].
type ContentChangedError struct {
	Changes []ContentChange
}

func (e *ContentChangedError) Error() string {
	listed := e.Changes
	var more string
	if len(listed) > maxMissingContentListed {
		listed = listed[:maxMissingContentListed]
		more = fmt.Sprintf(" and %d more", len(e.Changes)-maxMissingContentListed)
	}
	descs := make([]string, len(listed))
	for i, change := range listed {
		descs[i] = change.String()
	}
	return fmt.Sprintf("%s: %d file(s): %s%s", ErrContentChanged, len(e.Changes), strings.Join(descs, ", "), more)
}

func (e *ContentChangedError) Unwrap() error { return ErrContentChanged }

// ContentChange describes a file that content was added from that is missing
// or whose size or content token has changed.
type ContentChange struct {
	Digest   string    // digest of the content
	FS       ocflfs.FS // FS the file is in; nil if the FS isn't open
	Path     string    // path of the file in FS
	Size     int64     // size when the file was added; -1 if unknown
	Token    string    // content token when the file was added; "" if unknown
	Missing  bool      // the file doesn't exist, or the FS isn't open
	NewSize  int64     // size of the file now, if it isn't missing
	NewToken string    // content token now; "" if unknown or missing
}

// String returns the file's path and what has changed.
func (c ContentChange) String() string {
	switch {
	case c.Missing:
		return fmt.Sprintf("%q is missing", c.Path)
	case c.Size >= 0 && c.NewSize >= 0 && c.NewSize != c.Size:
		return fmt.Sprintf("%q has size %d, not %d", c.Path, c.NewSize, c.Size)
	default:
		return fmt.Sprintf("%q has changed since it was added (%s is now %s)", c.Path, c.Token, c.NewToken)
	}
}
