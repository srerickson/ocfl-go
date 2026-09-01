package config_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/config"
	ocflhttp "github.com/srerickson/ocfl-go/fs/http"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/fs/s3"
)

// stubS3API is an s3.S3API that is not an *awss3.Client: it carries none of
// the settings that an s3:// url can express.
type stubS3API struct{ s3.S3API }

// fileURL returns the file url for a path in the local file system.
func fileURL(dir string) string {
	p := filepath.ToSlash(dir)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String()
}

// noIMDS keeps the AWS configuration loader from reaching for the instance
// metadata service while resolving a region.
func noIMDS(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func TestNew(t *testing.T) {
	noIMDS(t)
	ctx := context.Background()
	tmpDir := t.TempDir()
	absTmpDir, err := filepath.Abs(tmpDir)
	be.NilErr(t, err)

	type testCase struct {
		name  string
		conf  string
		opts  []config.Option
		check func(t *testing.T, cnf *config.FSConfig)
	}
	testCases := []testCase{
		{
			name: "local path",
			conf: tmpDir,
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*local.FS)
				be.True(t, ok)
				be.Equal(t, absTmpDir, fsys.Root())
				be.Equal(t, ".", cnf.Path)
			},
		},
		{
			name: "relative local path",
			conf: ".",
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*local.FS)
				be.True(t, ok)
				abs, err := filepath.Abs(".")
				be.NilErr(t, err)
				be.Equal(t, abs, fsys.Root())
				be.Equal(t, ".", cnf.Path)
			},
		},
		{
			name: "file url",
			conf: fileURL(tmpDir),
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*local.FS)
				be.True(t, ok)
				be.Equal(t, absTmpDir, fsys.Root())
				be.Equal(t, ".", cnf.Path)
			},
		},
		{
			name: "file url with localhost",
			conf: strings.Replace(fileURL(tmpDir), "file://", "file://localhost", 1),
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*local.FS)
				be.True(t, ok)
				be.Equal(t, absTmpDir, fsys.Root())
				be.Equal(t, ".", cnf.Path)
			},
		},
		{
			name: "s3 bucket",
			conf: "s3://bucket?region=us-east-1",
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*s3.BucketFS)
				be.True(t, ok)
				be.Equal(t, "bucket", fsys.Bucket())
				be.Equal(t, ".", cnf.Path)
			},
		},
		{
			name: "s3 bucket with path and settings",
			conf: "s3://bucket/prefix/dir?region=us-west-2&endpoint=http://localhost:9000&path-style=true",
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*s3.BucketFS)
				be.True(t, ok)
				be.Equal(t, "bucket", fsys.Bucket())
				be.Equal(t, "prefix/dir", cnf.Path)
				client, ok := fsys.Client().(*awss3.Client)
				be.True(t, ok)
				opts := client.Options()
				be.Equal(t, "us-west-2", opts.Region)
				be.Equal(t, "http://localhost:9000", *opts.BaseEndpoint)
				be.True(t, opts.UsePathStyle)
			},
		},
		{
			name: "s3 bucket with trailing slash",
			conf: "s3://bucket/prefix/?region=us-east-1",
			check: func(t *testing.T, cnf *config.FSConfig) {
				be.Equal(t, "prefix", cnf.Path)
			},
		},
		{
			name: "s3 bucket with client option",
			conf: "s3://bucket/prefix",
			opts: []config.Option{config.WithS3Client(stubS3API{})},
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*s3.BucketFS)
				be.True(t, ok)
				_, ok = fsys.Client().(stubS3API)
				be.True(t, ok)
				be.Equal(t, "prefix", cnf.Path)
			},
		},
		{
			name: "https url",
			conf: "https://example.org/ocfl",
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*ocflhttp.FS)
				be.True(t, ok)
				be.Equal(t, "https://example.org/ocfl", fsys.BaseURL())
				be.Equal(t, ".", cnf.Path)
			},
		},
		{
			name: "http url with client option",
			conf: "http://example.org/ocfl",
			opts: []config.Option{config.WithHTTPClient(&http.Client{})},
			check: func(t *testing.T, cnf *config.FSConfig) {
				fsys, ok := cnf.FS.(*ocflhttp.FS)
				be.True(t, ok)
				be.Equal(t, "http://example.org/ocfl", fsys.BaseURL())
				be.True(t, fsys.Client() != nil)
				be.Equal(t, ".", cnf.Path)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cnf, err := config.New(ctx, tc.conf, tc.opts...)
			be.NilErr(t, err)
			tc.check(t, cnf)
			// the embedded FS makes the config itself usable as an FS.
			var _ ocflfs.FS = cnf
		})
	}
}

func TestNewErrors(t *testing.T) {
	noIMDS(t)
	ctx := context.Background()
	testCases := []struct {
		name string
		conf string
	}{
		{name: "empty string", conf: ""},
		{name: "unsupported scheme", conf: "ftp://example.org/ocfl"},
		{name: "missing local directory", conf: "./no-such-directory"},
		{name: "file url without path", conf: "file://"},
		{name: "file url with host", conf: "file://example.org/srv/ocfl"},
		{name: "file url with query", conf: "file:///srv/ocfl?region=us-east-1"},
		{name: "relative file url", conf: "file:srv/ocfl"},
		{name: "s3 url without bucket", conf: "s3:///prefix"},
		{name: "s3 url with unknown parameter", conf: "s3://bucket?path_style=true"},
		{name: "s3 url with invalid path-style", conf: "s3://bucket?path-style=maybe"},
		{name: "http url with query", conf: "https://example.org/ocfl?region=us-east-1"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cnf, err := config.New(ctx, tc.conf)
			be.True(t, err != nil)
			be.True(t, cnf == nil)
		})
	}
}

// sameBackend reports whether the two configurations name the same storage
// backend, per [ocflfs.SameBackend].
func sameBackend(t *testing.T, a, b *config.FSConfig) bool {
	t.Helper()
	sb, ok := a.FS.(ocflfs.SameBackend)
	be.True(t, ok)
	return sb.SameBackend(b.FS)
}

func TestSameBackend(t *testing.T) {
	noIMDS(t)
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("s3 settings decide, not the string", func(t *testing.T) {
		testCases := []struct {
			name   string
			a, b   string
			expect bool
		}{
			{
				name:   "identical strings",
				a:      "s3://bucket/prefix?region=us-east-1",
				b:      "s3://bucket/prefix?region=us-east-1",
				expect: true,
			},
			{
				name:   "same bucket, different prefix",
				a:      "s3://bucket?region=us-east-1",
				b:      "s3://bucket/prefix/dir?region=us-east-1",
				expect: true,
			},
			{
				name:   "same settings written in a different order",
				a:      "s3://bucket?region=us-west-2&endpoint=http://localhost:9000&path-style=true",
				b:      "s3://bucket?path-style=true&endpoint=http://localhost:9000&region=us-west-2",
				expect: true,
			},
			{
				name:   "different bucket",
				a:      "s3://bucket?region=us-east-1",
				b:      "s3://other?region=us-east-1",
				expect: false,
			},
			{
				name:   "different region",
				a:      "s3://bucket?region=us-east-1",
				b:      "s3://bucket?region=us-west-2",
				expect: false,
			},
			{
				name:   "different endpoint",
				a:      "s3://bucket?region=us-east-1",
				b:      "s3://bucket?region=us-east-1&endpoint=http://localhost:9000",
				expect: false,
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				a, err := config.New(ctx, tc.a)
				be.NilErr(t, err)
				b, err := config.New(ctx, tc.b)
				be.NilErr(t, err)
				be.Equal(t, tc.expect, sameBackend(t, a, b))
				be.Equal(t, tc.expect, sameBackend(t, b, a))
			})
		}
	})

	t.Run("s3 clients are reused", func(t *testing.T) {
		conf := "s3://bucket/prefix?region=eu-central-1"
		a, err := config.New(ctx, conf)
		be.NilErr(t, err)
		b, err := config.New(ctx, conf)
		be.NilErr(t, err)
		be.Equal(t, a.FS.(*s3.BucketFS).Client(), b.FS.(*s3.BucketFS).Client())
	})

	t.Run("s3 clients are reused across concurrent calls", func(t *testing.T) {
		conf := "s3://bucket?region=ap-south-1"
		clients := make([]s3.S3API, 8)
		var wg sync.WaitGroup
		for i := range clients {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cnf, err := config.New(ctx, conf)
				if err != nil {
					t.Error(err)
					return
				}
				clients[i] = cnf.FS.(*s3.BucketFS).Client()
			}()
		}
		wg.Wait()
		for _, client := range clients {
			be.Equal(t, clients[0], client)
		}
	})

	t.Run("s3 round trip through ambient aws configuration", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-gov-west-1")
		t.Setenv("AWS_ENDPOINT_URL_S3", "http://ambient.example:9000")

		// This configuration string names only the bucket: its region and
		// endpoint come from the environment. It is the only such string in
		// these tests, which matters because the client it resolves to is
		// cached for the life of the process.
		ambient, err := config.New(ctx, "s3://bucket/prefix")
		be.NilErr(t, err)
		text, err := ambient.MarshalText()
		be.NilErr(t, err)
		// marshaling writes out what the environment resolved to.
		be.Equal(t, "s3://bucket/prefix?endpoint=http%3A%2F%2Fambient.example%3A9000&region=us-gov-west-1", string(text))

		var back config.FSConfig
		be.NilErr(t, back.UnmarshalText(text))
		be.Equal(t, ambient.Path, back.Path)
		be.True(t, sameBackend(t, ambient, &back))

		// the settings are all spelled out now, so marshaling again is a
		// no-op.
		again, err := back.MarshalText()
		be.NilErr(t, err)
		be.Equal(t, string(text), string(again))
	})

	t.Run("s3 ambient settings join a client already built", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-gov-east-1")
		t.Setenv("AWS_ENDPOINT_URL_S3", "http://joined.example:9000")

		// the same two settings, once spelled out and once left to the
		// environment, in that order. path-style keeps this pair's cache
		// entries to itself.
		explicit, err := config.New(ctx, "s3://bucket?path-style=true&region=us-gov-east-1&endpoint=http://joined.example:9000")
		be.NilErr(t, err)
		ambient, err := config.New(ctx, "s3://bucket?path-style=true")
		be.NilErr(t, err)
		be.True(t, sameBackend(t, explicit, ambient))

		explicitText, err := explicit.MarshalText()
		be.NilErr(t, err)
		ambientText, err := ambient.MarshalText()
		be.NilErr(t, err)
		be.Equal(t, string(explicitText), string(ambientText))
	})

	t.Run("s3 with a shared client", func(t *testing.T) {
		client := awss3.NewFromConfig(aws.Config{Region: "us-east-1"})
		cnf, err := config.New(ctx, "s3://bucket/prefix", config.WithS3Client(client))
		be.NilErr(t, err)
		// a bucket fs built by hand shares the backend when it shares the
		// client, and not otherwise: the client New would have built is a
		// different one.
		shared := &config.FSConfig{FS: s3.NewBucketFS(client, "bucket"), Path: "."}
		be.True(t, sameBackend(t, cnf, shared))
		own, err := config.New(ctx, "s3://bucket/prefix?region=us-east-1")
		be.NilErr(t, err)
		be.False(t, sameBackend(t, cnf, own))
	})

	t.Run("local paths", func(t *testing.T) {
		a, err := config.New(ctx, tmpDir)
		be.NilErr(t, err)
		b, err := config.New(ctx, fileURL(tmpDir))
		be.NilErr(t, err)
		be.True(t, sameBackend(t, a, b))
		other, err := config.New(ctx, t.TempDir())
		be.NilErr(t, err)
		be.False(t, sameBackend(t, a, other))
	})
}

func TestFSConfigRoundTrip(t *testing.T) {
	noIMDS(t)
	ctx := context.Background()
	tmpDir := t.TempDir()
	absTmpDir, err := filepath.Abs(tmpDir)
	be.NilErr(t, err)

	testCases := []struct {
		name string
		conf string
		want string // canonical form; the same as conf unless noted
	}{
		{
			name: "local path",
			conf: tmpDir,
			want: fileURL(absTmpDir),
		},
		{
			name: "file url",
			conf: fileURL(tmpDir),
			want: fileURL(absTmpDir),
		},
		{
			name: "s3 bucket",
			conf: "s3://bucket?region=us-east-1",
			want: "s3://bucket?region=us-east-1",
		},
		{
			name: "s3 bucket with path",
			conf: "s3://bucket/prefix/dir?region=us-east-1",
			want: "s3://bucket/prefix/dir?region=us-east-1",
		},
		{
			name: "s3 bucket with all settings",
			conf: "s3://bucket/prefix?region=us-west-2&endpoint=http://localhost:9000&path-style=true",
			want: "s3://bucket/prefix?endpoint=http%3A%2F%2Flocalhost%3A9000&path-style=true&region=us-west-2",
		},
		{
			name: "https url",
			conf: "https://example.org/ocfl",
			want: "https://example.org/ocfl",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cnf, err := config.New(ctx, tc.conf)
			be.NilErr(t, err)
			text, err := cnf.MarshalText()
			be.NilErr(t, err)
			be.Equal(t, tc.want, string(text))

			// unmarshaling the result and marshaling it again is stable.
			var next config.FSConfig
			be.NilErr(t, next.UnmarshalText(text))
			be.Equal(t, cnf.Path, next.Path)
			nextText, err := next.MarshalText()
			be.NilErr(t, err)
			be.Equal(t, string(text), string(nextText))
		})
	}
}

func TestFSConfigMarshalText(t *testing.T) {
	t.Run("s3 client without settings", func(t *testing.T) {
		cnf := config.FSConfig{FS: s3.NewBucketFS(stubS3API{}, "bucket")}
		text, err := cnf.MarshalText()
		be.NilErr(t, err)
		be.Equal(t, "s3://bucket", string(text))
	})
	t.Run("s3 client with path", func(t *testing.T) {
		client := awss3.NewFromConfig(aws.Config{Region: "us-east-1"})
		cnf := config.FSConfig{FS: s3.NewBucketFS(client, "bucket"), Path: "prefix/dir"}
		text, err := cnf.MarshalText()
		be.NilErr(t, err)
		be.Equal(t, "s3://bucket/prefix/dir?region=us-east-1", string(text))
	})
	t.Run("fs without MarshalText", func(t *testing.T) {
		cnf := config.FSConfig{FS: ocflfs.DirFS("."), Path: "."}
		_, err := cnf.MarshalText()
		be.True(t, err != nil)
	})
	t.Run("missing fs", func(t *testing.T) {
		cnf := config.FSConfig{Path: "."}
		_, err := cnf.MarshalText()
		be.True(t, err != nil)
	})
	t.Run("invalid path", func(t *testing.T) {
		cnf := config.FSConfig{FS: s3.NewBucketFS(stubS3API{}, "bucket"), Path: "../up"}
		_, err := cnf.MarshalText()
		be.True(t, err != nil)
	})
	t.Run("http fs with invalid base url", func(t *testing.T) {
		cnf := config.FSConfig{FS: ocflhttp.New("example.org/ocfl"), Path: "."}
		_, err := cnf.MarshalText()
		be.True(t, err != nil)
	})
}

func TestFSConfigJSON(t *testing.T) {
	noIMDS(t)
	type storeSettings struct {
		Store config.FSConfig `json:"store"`
	}
	settings := storeSettings{}
	be.NilErr(t, json.Unmarshal([]byte(`{"store":"s3://bucket/prefix?region=us-east-1"}`), &settings))
	fsys, ok := settings.Store.FS.(*s3.BucketFS)
	be.True(t, ok)
	be.Equal(t, "bucket", fsys.Bucket())
	be.Equal(t, "prefix", settings.Store.Path)

	out, err := json.Marshal(settings)
	be.NilErr(t, err)
	be.Equal(t, `{"store":"s3://bucket/prefix?region=us-east-1"}`, string(out))
}

func TestFSConfigUnmarshalTextError(t *testing.T) {
	var cnf config.FSConfig
	be.True(t, cnf.UnmarshalText([]byte("ftp://example.org/ocfl")) != nil)
	be.True(t, cnf.FS == nil)
}
