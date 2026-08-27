package s3

import "strings"

// unreservedCopySource reports whether b may appear literally in the
// x-amz-copy-source header. It is the RFC 3986 unreserved set plus "/",
// which S3 needs literal: once to separate the bucket from the key, and
// again for every directory separator within the key.
func unreservedCopySource(b byte) bool {
	switch {
	case 'A' <= b && b <= 'Z', 'a' <= b && b <= 'z', '0' <= b && b <= '9':
		return true
	}
	switch b {
	case '-', '_', '.', '~', '/':
		return true
	}
	return false
}

const upperhex = "0123456789ABCDEF"

// encodeCopySource builds the value for the x-amz-copy-source header:
// "<bucket>/<key>" with "/" left literal and every other byte outside the
// RFC 3986 unreserved set percent-encoded.
//
// net/url has no function with these semantics. QueryEscape encodes "/" as
// %2F -- destroying both the bucket/key separator and every path separator
// in the key -- and writes a space as "+", which S3 reads literally.
// PathEscape encodes "/" too. The AWS SDK does not encode the value for us:
// it sets the header verbatim and documents that "the value must be
// URL-encoded", leaving the encoding to the caller.
//
// key must be a raw OCFL path, never an already-encoded one: "%" is itself
// encoded, so encoding twice yields the wrong key.
func encodeCopySource(bucket, key string) string {
	src := bucket + "/" + key
	// Iterate bytes, not runes: each byte of a multi-byte UTF-8 rune becomes
	// its own %XX, which is what S3 expects.
	n := 0
	for i := range len(src) {
		if !unreservedCopySource(src[i]) {
			n++
		}
	}
	if n == 0 {
		return src
	}
	var b strings.Builder
	b.Grow(len(src) + 2*n)
	for i := range len(src) {
		c := src[i]
		if unreservedCopySource(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperhex[c>>4])
		b.WriteByte(upperhex[c&0x0f])
	}
	return b.String()
}
