package s3

import (
	"context"
	"io/fs"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func remove(ctx context.Context, api RemoveAPI, b string, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("remove", name, fs.ErrInvalid)
	}
	if name == "." {
		return pathErr("remove", name, fs.ErrNotExist)
	}
	// Contract (WriteFS.Remove in fs/fs.go): removing a missing file must
	// return an error satisfying errors.Is(err, fs.ErrNotExist). S3's
	// DeleteObject is idempotent — it succeeds (204) even for missing keys —
	// so probe existence with HeadObject first and map a not-found HEAD to
	// fs.ErrNotExist before deleting.
	if _, err := api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &b,
		Key:    aws.String(name),
	}); err != nil {
		if errIsNotExist(err) {
			return pathErr("remove", name, notExistErr(err))
		}
		return pathErr("remove", name, err)
	}
	if _, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &b,
		Key:    aws.String(name),
	}); err != nil {
		return pathErr("remove", name, err)
	}
	return nil
}
