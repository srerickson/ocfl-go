package s3

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func removeAll(ctx context.Context, api RemoveAllAPI, buck string, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("removeall", name, fs.ErrInvalid)
	}
	params := &s3.ListObjectsV2Input{Bucket: &buck, MaxKeys: &maxKeys}
	if name != "." {
		params.Prefix = aws.String(name + "/")
	}
	for {
		list, err := api.ListObjectsV2(ctx, params)
		if err != nil {
			return pathErr("removeall", name, err)
		}
		// Delete each page of listed objects with a single batch
		// DeleteObjects request.
		if len(list.Contents) > 0 {
			identifiers := make([]types.ObjectIdentifier, 0, len(list.Contents))
			for _, obj := range list.Contents {
				identifiers = append(identifiers, types.ObjectIdentifier{Key: obj.Key})
			}
			out, err := api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: &buck,
				Delete: &types.Delete{Objects: identifiers},
			})
			if err != nil {
				return pathErr("removeall", name, err)
			}
			// S3 returns HTTP 200 even when individual keys in the batch
			// fail to delete; the per-key failures are reported in the
			// response body's Errors list. A successful API call therefore
			// does not by itself mean all objects were deleted, so surface
			// any partial failures as a joined PathError.
			if out != nil && len(out.Errors) > 0 {
				collected := make([]error, 0, len(out.Errors))
				for _, e := range out.Errors {
					collected = append(collected,
						fmt.Errorf("key %q: %s", aws.ToString(e.Key), aws.ToString(e.Message)))
				}
				return pathErr("removeAll", name, errors.Join(collected...))
			}
		}
		params.ContinuationToken = list.NextContinuationToken
		if params.ContinuationToken == nil {
			break
		}
	}
	return nil
}
