package testfs

import (
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/cloud"
	"gocloud.dev/blob/memblob"
)

type MemFS struct {
	ocfl.WriteFS
}

func NewMemFS() *MemFS {
	return &MemFS{
		WriteFS: cloud.NewFS(memblob.OpenBucket(nil)),
	}
}
