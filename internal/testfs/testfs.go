package testfs

import (
	"github.com/srerickson/ocfl/backend/cloud"
	"github.com/srerickson/ocfl/logger"
	"gocloud.dev/blob/memblob"
)

type MemFS struct {
	*cloud.FS
}

func NewMemFS() *MemFS {
	return &MemFS{
		FS: cloud.NewFS(memblob.OpenBucket(nil), cloud.WithLogger(logger.DefaultLogger())),
	}
}
