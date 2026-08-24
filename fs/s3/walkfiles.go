package s3

import (
	"context"
	"io/fs"
	"iter"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// walkFiles returns an iterator that yields FileRefs for files in the dir.
// Each yielded FileRef carries fsys in its FS field, matching the contract
// of the local fileWalk path (fs/fs.go), so callers can open files through
// the ref without holding the backend separately.
func walkFiles(ctx context.Context, fsys ocflfs.FS, api FilesAPI, buck string, dir string) iter.Seq2[*ocflfs.FileRef, error] {
	return func(yield func(*ocflfs.FileRef, error) bool) {
		const op = "list_files"
		if !fs.ValidPath(dir) {
			yield(nil, pathErr(op, dir, fs.ErrInvalid))
			return
		}
		params := &s3.ListObjectsV2Input{
			Bucket:  &buck,
			MaxKeys: &maxKeys,
		}
		if dir != "." {
			params.Prefix = aws.String(dir + "/")
		}
		for {
			listPage, err := api.ListObjectsV2(ctx, params)
			if err != nil {
				yield(nil, pathErr(op, dir, err))
				return
			}
			for _, s3obj := range listPage.Contents {
				// A non-AWS FilesAPI implementation may return partial
				// entries whose Key is nil. Such objects cannot be
				// addressed and must be skipped; dereferencing the nil
				// pointer below would panic inside the iterator instead
				// of returning an error.
				if s3obj.Key == nil {
					continue
				}
				// skip S3 directory placeholder objects: zero-byte keys ending
				// with "/" created by the S3 console or some clients to
				// represent directories. They are not files and would
				// otherwise appear as phantom empty files in OCFL inventories.
				// This also skips the directory prefix's own placeholder
				// (e.g. "dir/" when listing under prefix "dir/").
				if strings.HasSuffix(*s3obj.Key, "/") {
					continue
				}
				refPath := *s3obj.Key
				if dir != "." {
					refPath = strings.TrimPrefix(refPath, dir+"/")
				}
				info := &ocflfs.FileRef{
					FS:      fsys,
					BaseDir: dir,
					Path:    refPath,
					Info: &iofsInfo{
						name:    path.Base(*s3obj.Key),
						size:    aws.ToInt64(s3obj.Size),
						mode:    fileMode,
						modTime: aws.ToTime(s3obj.LastModified),
					},
				}
				if !yield(info, nil) {
					return
				}
			}
			params.ContinuationToken = listPage.NextContinuationToken
			if params.ContinuationToken == nil {
				break
			}
		}
	}
}
