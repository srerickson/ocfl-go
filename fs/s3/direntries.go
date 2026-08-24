package s3

import (
	"context"
	"io/fs"
	"iter"
	"path"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// dirEntries implements directory listing: the emulation of fs.ReadDir on
// S3's flat key space. It is the S3 half of the backend readdir contract
// pinned by fs/s3/direntries_test.go and fs/local/localfs_test.go; read the
// two together.
//
// # S3 has no directories
//
// S3 stores only objects; a "directory" is an emergent property of key
// prefixes (a prefix with a trailing "/" used with a Delimiter), and there
// is no way to create an empty one. Every branch below follows from that:
//
//   - A prefix that has objects or deeper common prefixes lists them as
//     file entries (fileMode) and subdirectory entries (dirMode),
//     respectively.
//   - A non-root prefix with neither objects nor common prefixes is
//     indistinguishable from a path that never existed — S3 offers no
//     "empty directory" object to tell the two apart — so it is reported as
//     missing with fs.ErrNotExist, matching the local backend's readdir of
//     a missing directory.
//   - The root, dir=".", is the one prefix that is always known to exist:
//     it is the bucket itself, and a *missing* bucket surfaces as a
//     ListObjectsV2 error rather than as an empty listing. On a bucket with
//     no keys at all, "." therefore yields zero entries and no error,
//     matching the local backend's readdir of an existing but empty
//     directory (fs/local/localfs_test.go, "empty top-level directory
//     returns zero entries").
//
// # Why the asymmetry is deliberate
//
// The two backends still disagree on empty *non-root* prefixes: a local
// empty directory reads back as an empty listing, while an S3 prefix that
// would occupy the same position reads back as fs.ErrNotExist. This is a
// faithful emulation, not a bug: local storage can represent emptiness, S3
// cannot, and a valid OCFL object never depends on an empty directory —
// every OCFL version directory contains at least inventory.json, and OCFL
// storage-root and object layouts do not create empty directories. OCFL
// extensions or third-party tooling may create empty directory-like
// prefixes on local storage; when an object is moved to S3 the same path
// simply reads as missing, which reflects the key space it now lives in.
//
// The root case is the only one where backends must agree, because
// storage-root scanning (Root.NewRoot in root.go) and ocflfs.RemoveAll(".")
// (fs/fs.go) both start by reading dir="." : an empty bucket is a valid
// (new) storage root and must read back as an empty directory, never as
// fs.ErrNotExist. (NewRoot in root.go happens to tolerate fs.ErrNotExist,
// but ocflfs.RemoveAll(".") does not — on an empty bucket it would
// otherwise fail instead of being the no-op it is on local storage — and
// "empty" is simply the honest answer for a prefix that is guaranteed to
// exist.) Root-empty behavior is pinned by TestDirEntries_RootEmptyBucket
// in fs/s3/direntries_test.go.
func dirEntries(ctx context.Context, api ReadDirAPI, buck string, dir string) iter.Seq2[fs.DirEntry, error] {
	return func(yield func(fs.DirEntry, error) bool) {
		if !fs.ValidPath(dir) {
			yield(nil, pathErr("readdir", dir, fs.ErrInvalid))
			return
		}
		params := &s3.ListObjectsV2Input{
			Bucket:    &buck,
			Delimiter: &delim,
			MaxKeys:   &maxKeys,
		}
		if dir != "." {
			params.Prefix = aws.String(dir + "/")
		}
		prefixHasContent := false
		for {
			list, err := api.ListObjectsV2(ctx, params)
			if err != nil {
				yield(nil, pathErr("readdir", dir, err))
				return
			}
			numDirs := len(list.CommonPrefixes)
			numFiles := len(list.Contents)
			numEntries := numDirs + numFiles
			if numEntries == 0 {
				if !prefixHasContent && dir != "." {
					// treat prefix without objects as a missing directory.
					// The root (dir=".") is exempt: "." names the bucket
					// itself, which always exists, so an empty bucket reads
					// as an empty directory rather than a missing path.
					yield(nil, pathErr("readdir", dir, fs.ErrNotExist))
				}
				return
			}
			prefixHasContent = true
			entries := make([]fs.DirEntry, numEntries)
			for i, item := range list.CommonPrefixes {
				entries[i] = &iofsInfo{
					name: path.Base(*item.Prefix),
					mode: dirMode,
				}
			}
			for i, item := range list.Contents {
				entries[numDirs+i] = &iofsInfo{
					name:    path.Base(*item.Key),
					size:    *item.Size,
					mode:    fileMode,
					modTime: *item.LastModified,
					//sys:     &item,
				}
			}
			slices.SortFunc(entries, func(a, b fs.DirEntry) int {
				return strings.Compare(a.Name(), b.Name())
			})
			for _, entry := range entries {
				if !yield(entry, nil) {
					return
				}
			}
			params.ContinuationToken = list.NextContinuationToken
			if params.ContinuationToken == nil {
				break
			}
		}
	}

}
