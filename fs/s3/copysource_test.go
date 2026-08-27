package s3

import "testing"

func TestEncodeCopySource(t *testing.T) {
	cases := []struct {
		desc        string
		bucket, key string
		expect      string
	}{
		{
			desc:   "plain key",
			bucket: "my-bucket",
			key:    "object.txt",
			expect: "my-bucket/object.txt",
		},
		{
			desc:   "nested slashes preserved literally",
			bucket: "my-bucket",
			key:    "a/b/c/object.txt",
			expect: "my-bucket/a/b/c/object.txt",
		},
		{
			desc:   "space encodes to %20, not +",
			bucket: "my-bucket",
			key:    "a file.txt",
			expect: "my-bucket/a%20file.txt",
		},
		{
			desc:   "plus encodes to %2B",
			bucket: "my-bucket",
			key:    "a+b.txt",
			expect: "my-bucket/a%2Bb.txt",
		},
		{
			desc:   "non-ASCII encodes per UTF-8 byte, uppercase hex",
			bucket: "my-bucket",
			key:    "café.txt",
			// 'é' is the two UTF-8 bytes 0xC3 0xA9.
			expect: "my-bucket/caf%C3%A9.txt",
		},
		{
			desc:   "CJK characters encode per UTF-8 byte",
			bucket: "my-bucket",
			key:    "日本語.txt",
			expect: "my-bucket/%E6%97%A5%E6%9C%AC%E8%AA%9E.txt",
		},
		{
			desc:   "unreserved punctuation left alone",
			bucket: "my-bucket",
			key:    "a-b_c.d~e",
			expect: "my-bucket/a-b_c.d~e",
		},
		{
			desc:   "percent sign encodes to %25 (no double-encoding)",
			bucket: "my-bucket",
			key:    "100%done.txt",
			expect: "my-bucket/100%25done.txt",
		},
		{
			desc:   "empty key",
			bucket: "my-bucket",
			key:    "",
			expect: "my-bucket/",
		},
	}
	for _, tcase := range cases {
		t.Run(tcase.desc, func(t *testing.T) {
			got := encodeCopySource(tcase.bucket, tcase.key)
			if got != tcase.expect {
				t.Errorf("encodeCopySource(%q, %q) = %q, want %q", tcase.bucket, tcase.key, got, tcase.expect)
			}
		})
	}
}
