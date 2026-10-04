package fs_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

func TestRegistry(t *testing.T) {
	ctx := context.Background()
	// opener returns an OpenFunc that records the configuration string it
	// was called with in *got.
	opener := func(got *string) ocflfs.OpenFunc {
		return func(_ context.Context, conf string) (ocflfs.FS, error) {
			*got = conf
			return ocflfs.NewWrapFS(fstest.MapFS{}), nil
		}
	}
	t.Run("zero value", func(t *testing.T) {
		var reg ocflfs.Registry
		be.Equal(t, 0, len(reg.Schemes()))
		_, err := reg.Open(ctx, "file:///tmp")
		be.True(t, errors.Is(err, ocflfs.ErrUnknownScheme))
	})
	t.Run("open", func(t *testing.T) {
		var gotFile, gotS3 string
		reg := ocflfs.NewRegistry(map[string]ocflfs.OpenFunc{
			"file": opener(&gotFile),
			"S3":   opener(&gotS3),
		})
		be.DeepEqual(t, []string{"file", "s3"}, reg.Schemes())
		fsys, err := reg.Open(ctx, "file:///tmp/dir")
		be.NilErr(t, err)
		be.Nonzero(t, fsys)
		be.Equal(t, "file:///tmp/dir", gotFile)
		// schemes are case-insensitive
		_, err = reg.Open(ctx, "s3://bucket")
		be.NilErr(t, err)
		be.Equal(t, "s3://bucket", gotS3)
		_, err = reg.Open(ctx, "FILE:///tmp/other")
		be.NilErr(t, err)
		be.Equal(t, "FILE:///tmp/other", gotFile)
	})
	t.Run("unknown scheme", func(t *testing.T) {
		var got string
		reg := ocflfs.NewRegistry(map[string]ocflfs.OpenFunc{"file": opener(&got)})
		for _, conf := range []string{"https://example.org", "relative/path", ""} {
			_, err := reg.Open(ctx, conf)
			be.True(t, errors.Is(err, ocflfs.ErrUnknownScheme))
		}
		be.Zero(t, got)
	})
	t.Run("open error", func(t *testing.T) {
		openErr := errors.New("can't open")
		reg := ocflfs.NewRegistry(map[string]ocflfs.OpenFunc{
			"file": func(context.Context, string) (ocflfs.FS, error) { return nil, openErr },
		})
		_, err := reg.Open(ctx, "file:///tmp")
		be.True(t, errors.Is(err, openErr))
	})
	t.Run("append", func(t *testing.T) {
		var gotOld, gotNew, gotHTTP string
		reg := ocflfs.NewRegistry(map[string]ocflfs.OpenFunc{"file": opener(&gotOld)})
		appended := reg.Append("FILE", opener(&gotNew)).Append("http", opener(&gotHTTP))
		be.DeepEqual(t, []string{"file", "http"}, appended.Schemes())
		_, err := appended.Open(ctx, "file:///tmp")
		be.NilErr(t, err)
		be.Equal(t, "file:///tmp", gotNew)
		be.Zero(t, gotOld)
		// r is unchanged
		be.DeepEqual(t, []string{"file"}, reg.Schemes())
		_, err = reg.Open(ctx, "file:///tmp")
		be.NilErr(t, err)
		be.Equal(t, "file:///tmp", gotOld)
		// Append works on the zero value
		var zero ocflfs.Registry
		be.DeepEqual(t, []string{"http"}, zero.Append("http", opener(&gotHTTP)).Schemes())
	})
	t.Run("changing the map doesn't change the registry", func(t *testing.T) {
		var got string
		openers := map[string]ocflfs.OpenFunc{"file": opener(&got)}
		reg := ocflfs.NewRegistry(openers)
		delete(openers, "file")
		be.DeepEqual(t, []string{"file"}, reg.Schemes())
	})
}
