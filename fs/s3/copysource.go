package s3

import "strings"

// copySourcePath returns the URL-encoded value for the x-amz-copy-source
// header used to copy src to dst in the same bucket.
//
// S3 requires the header to be "<bucket>/<key>" with the '/' separators left
// literal and the value URL-encoded (see the CopyObject API reference). Each
// path segment is therefore percent-encoded individually: every byte outside
// the RFC 3986 unreserved set (A-Za-z0-9-_.~) is encoded as an uppercase %XX
// pair (space -> %20, '+' -> %2B) and unencoded '/' is preserved as the
// segment and bucket/key separator.
//
// The key must be the raw, unencoded OCFL path; it is encoded exactly once
// here. Never pass an already-encoded value (that would double-encode '%').
func copySourcePath(buck, key string) string {
	return encodePath(buck) + "/" + encodePath(key)
}

// encodePath percent-encodes path, keeping '/' literal. It follows minio-go's
// s3utils.EncodePath (proven against AWS S3 and compatible stores for
// x-amz-copy-source): unlike url.QueryEscape it never emits '+' for space and
// never encodes the '/' separators, and unlike url.PathEscape it also escapes
// '+', '&', '=', '@', ':', '$' and the other sub-delims that must be encoded
// in a copy-source value.
func encodePath(path string) string {
	var b strings.Builder
	b.Grow(len(path))
	for i := 0; i < len(path); i++ {
		switch c := path[i]; {
		case c == '/':
			b.WriteByte(c)
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(upperhex[c>>4])
			b.WriteByte(upperhex[c&15])
		}
	}
	return b.String()
}

const upperhex = "0123456789ABCDEF"
