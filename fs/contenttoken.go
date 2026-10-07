package fs

import "io/fs"

// ContentTokener is an optional interface for an [fs.FileInfo] returned by an
// [FS]. ContentToken returns a string that changes whenever the file's
// content changes, or "" if the backend can't provide one.
//
// A token is opaque, but starts with a prefix naming its kind (for example,
// "etag:"), so that tokens of different kinds never match. Equal tokens must
// mean the file's content hasn't changed; different tokens needn't mean it
// has.
type ContentTokener interface {
	ContentToken() string
}

// ContentToken returns a content token for info: from its ContentToken method
// if it implements [ContentTokener], otherwise from an OS stat structure
// returned by info.Sys(). It returns "" if info is nil or no token is
// available.
//
// A token from an OS stat structure has the form "stat:<mtime>:<ctime>" on
// unix systems and "stat:<mtime>" on Windows, with times in nanoseconds since
// the Unix epoch. A file changed within the file system's timestamp
// resolution of when the token was read may keep the same token.
func ContentToken(info fs.FileInfo) string {
	if info == nil {
		return ""
	}
	if tokener, ok := info.(ContentTokener); ok {
		return tokener.ContentToken()
	}
	return sysContentToken(info.Sys())
}
