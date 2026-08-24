package s3_test

// Tests for fs.go: the interface set BucketFS claims to implement. The
// behavior behind each interface is tested in the file named for the
// operation that implements it (openfile_test.go, write_test.go, ...).

import (
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
)

var (
	_ ocflfs.FS           = (*s3.BucketFS)(nil)
	_ ocflfs.DirEntriesFS = (*s3.BucketFS)(nil)
	_ ocflfs.CopyFS       = (*s3.BucketFS)(nil)
	_ ocflfs.WriteFS      = (*s3.BucketFS)(nil)
	_ ocflfs.FileWalker   = (*s3.BucketFS)(nil)
)
