package ocfl

import (
	"errors"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// ErrContentChanged is returned by [ObjectUpdate.Apply] when content from its
// [ContentSource] doesn't match the content's digest: for example, because
// the file it was staged from has changed.
var ErrContentChanged = errors.New("content doesn't match its digest")

// ContentSource is used to access content with a given digest when creating and
// updating objects.
type ContentSource interface {
	// GetContent returns an FS and path to a file in FS for a file with the given digest.
	// If no content is associated with the digest, fsys is nil and path is an empty string.
	GetContent(digest string) (fsys ocflfs.FS, path string)
}
