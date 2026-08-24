package s3

// Internal tests for copysource.go: the exact wire value copySourcePath
// produces, which the external s3_test package cannot call.

import (
	"net/url"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
)

// TestCopySourcePath asserts the exact x-amz-copy-source value produced for
// bucket/key pairs. '/' separators (both the bucket/key separator and in-key
// path separators) must remain literal, spaces must become %20 (never '+'),
// and each segment must be percent-encoded exactly once.
func TestCopySourcePath(t *testing.T) {
	const buck = "ocfl-go-test"
	tests := []struct {
		desc string
		buck string
		key  string
		want string
	}{
		{
			desc: "plain key",
			key:  "src-file",
			want: "ocfl-go-test/src-file",
		}, {
			desc: "nested slashes stay literal",
			key:  "dir/sub/file.txt",
			want: "ocfl-go-test/dir/sub/file.txt",
		}, {
			desc: "space becomes %20 not +",
			key:  "my file.txt",
			want: "ocfl-go-test/my%20file.txt",
		}, {
			desc: "space in nested segments",
			key:  "dir/sub dir/file with space.txt",
			want: "ocfl-go-test/dir/sub%20dir/file%20with%20space.txt",
		}, {
			desc: "literal plus is escaped",
			key:  "file+plus.txt",
			want: "ocfl-go-test/file%2Bplus.txt",
		}, {
			desc: "percent is escaped once",
			key:  "100%.txt",
			want: "ocfl-go-test/100%25.txt",
		}, {
			desc: "literal %20 sequence is not double-decoded",
			key:  "file%20name.txt",
			want: "ocfl-go-test/file%2520name.txt",
		}, {
			desc: "hash and question mark are escaped",
			key:  "why#what?x",
			want: "ocfl-go-test/why%23what%3Fx",
		}, {
			desc: "unicode is UTF-8 percent-encoded",
			key:  "naïve-日本語.txt",
			want: "ocfl-go-test/na%C3%AFve-%E6%97%A5%E6%9C%AC%E8%AA%9E.txt",
		}, {
			desc: "emoji is UTF-8 percent-encoded",
			key:  "emoji-😀.txt",
			want: "ocfl-go-test/emoji-%F0%9F%98%80.txt",
		}, {
			desc: "query and sub-delims are escaped",
			key:  "a&b=c:d$e@f",
			want: "ocfl-go-test/a%26b%3Dc%3Ad%24e%40f",
		}, {
			desc: "gen-delims and other specials are escaped",
			key:  "semi;comma,parens()bang!star*quote'",
			want: "ocfl-go-test/semi%3Bcomma%2Cparens%28%29bang%21star%2Aquote%27",
		}, {
			desc: "control characters are escaped",
			key:  "tab\there.txt",
			want: "ocfl-go-test/tab%09here.txt",
		}, {
			desc: "unreserved characters stay literal",
			key:  "tilde~dot.-under_score",
			want: "ocfl-go-test/tilde~dot.-under_score",
		}, {
			desc: "bucket with dots and hyphens is preserved",
			buck: "my-bucket.name",
			key:  "dir/file.txt",
			want: "my-bucket.name/dir/file.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			b := tt.buck
			if b == "" {
				b = buck
			}
			got := copySourcePath(b, tt.key)
			be.Equal(t, tt.want, got)
			// No double-encoding: the raw bucket/key must come back
			// unchanged after a single decode (PathUnescape is what the
			// server-side, and this repo's S3 mock, applies).
			decoded, err := url.PathUnescape(got)
			be.NilErr(t, err)
			be.Equal(t, b+"/"+tt.key, decoded)
		})
	}
}

// TestCopySourcePathEncodingInvariants guards the two failure modes of the
// previous url.QueryEscape implementation: '+' for spaces (rejected by S3)
// and '%2F' for '/' separators (400 InvalidArgument on S3-compatible
// stores). Neither may ever appear in a copy-source value.
func TestCopySourcePathEncodingInvariants(t *testing.T) {
	keys := []string{
		"my file.txt",
		"dir/sub dir/file with space.txt",
		"file+plus.txt",
		"100%.txt",
		"file%2Fname.txt",
		"why#what?x",
		"naïve-日本語/emoji-😀.txt",
	}
	for _, key := range keys {
		got := copySourcePath("ocfl-go-test", key)
		if strings.Contains(got, "+") {
			t.Errorf("copy source contains '+': %q", got)
		}
		if strings.Contains(got, "%2F") {
			t.Errorf("copy source contains %%2F: %q", got)
		}
	}
}
