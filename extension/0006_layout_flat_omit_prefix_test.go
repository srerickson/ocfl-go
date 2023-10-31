package extension_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/extension"
)

func TestLayoutFlatOmitPrefix(t *testing.T) {
	type test struct {
		delim  string
		in     string
		expect string
		ok     bool
	}
	tests := []test{
		{delim: ":", in: "namespace:12887296", expect: "12887296", ok: true},
		{delim: ":", in: "urn:uuid:6e8bc430-9c3a-11d9-9669-0800200c9a66", expect: "6e8bc430-9c3a-11d9-9669-0800200c9a66", ok: true},
		{delim: "EDU/", in: "https://institution.edu/3448793", expect: "3448793", ok: true},
		{delim: "edu/", in: "https://institution.edu/abc/edu/f8.05v", expect: "f8.05v", ok: true},
		{delim: "info:", in: "https://example.org/info:/12345/x54xz321/s3/f8.05v", expect: "", ok: false},
	}
	for _, ts := range tests {
		l := extension.LayoutFlatOmitPrefix{}
		l.Delimiter = ts.delim
		got, err := l.Resolve(ts.in)
		if (err == nil) != ts.ok {
			if err == nil {
				t.Errorf("expected an error with input %q", ts.in)
			} else {
				t.Error()
			}
		}
		if got != ts.expect {
			t.Errorf("layout for id=%q, got=%q, expect=%q", ts.in, got, ts.expect)
		}
	}
}
