package memfs

import (
	"context"
	"io"

	"github.com/srerickson/ocfl-go/backend/cloud"
	"gocloud.dev/blob/memblob"
)

type FS struct {
	*cloud.FS
}

func New() *FS {
	return &FS{
		FS: cloud.NewFS(memblob.OpenBucket(nil)),
	}
}

func NewWith(cont map[string]io.Reader) (*FS, error) {
	ctx := context.Background()
	fsys := New()
	for p, reader := range cont {
		if _, err := fsys.Write(ctx, p, reader); err != nil {
			return nil, err
		}
		if closer, ok := reader.(io.Closer); ok {
			closer.Close()
		}
	}
	return fsys, nil
}
