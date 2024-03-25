package mock

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/backend/s3"
)

type Object struct {
	Key          string
	Body         []byte
	LastModified time.Time
}

func objectMap(objs ...*Object) map[string]*Object {
	mockObjects := make(map[string]*Object, len(objs))
	for _, obj := range objs {
		mockObjects[obj.Key] = obj
	}
	return mockObjects
}

func OpenFileAPI(t *testing.T, bucket string, objects ...*Object) s3.OpenFileAPI {
	mockObjects := objectMap(objects...)
	getObj := func(ctx context.Context, param *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
		be.Nonzero(t, param.Bucket)
		be.Nonzero(t, param.Key)
		if *param.Bucket != bucket {
			return nil, &types.NoSuchBucket{}
		}
		obj := mockObjects[*param.Key]
		if obj == nil {
			return nil, &types.NoSuchKey{}
		}
		return &s3v2.GetObjectOutput{
			Body:          io.NopCloser(bytes.NewBuffer(obj.Body)),
			ContentLength: aws.Int64(int64(len(obj.Body))),
			LastModified:  aws.Time(obj.LastModified),
		}, nil
	}
	return &s3API{get: getObj}
}

func ReadDirAPI(t *testing.T, bucket string, objects ...*Object) s3.ReadDirAPI {

}

type s3fn[input any, output any] func(context.Context, input, ...func(*s3v2.Options)) (output, error)

type s3API struct {
	head        s3fn[*s3v2.HeadObjectInput, *s3v2.HeadObjectOutput]
	get         s3fn[*s3v2.GetObjectInput, *s3v2.GetObjectOutput]
	list        s3fn[*s3v2.ListObjectsV2Input, *s3v2.ListObjectsV2Output]
	put         s3fn[*s3v2.PutObjectInput, *s3v2.PutObjectOutput]
	copy        s3fn[*s3v2.CopyObjectInput, *s3v2.CopyObjectOutput]
	delete      s3fn[*s3v2.DeleteObjectInput, *s3v2.DeleteObjectOutput]
	createMPU   s3fn[*s3v2.CreateMultipartUploadInput, *s3v2.CreateMultipartUploadOutput]
	completeMPU s3fn[*s3v2.CompleteMultipartUploadInput, *s3v2.CompleteMultipartUploadOutput]
	abortMPU    s3fn[*s3v2.AbortMultipartUploadInput, *s3v2.AbortMultipartUploadOutput]
	putPart     s3fn[*s3v2.UploadPartInput, *s3v2.UploadPartOutput]
	putPartCopy s3fn[*s3v2.UploadPartCopyInput, *s3v2.UploadPartCopyOutput]
}

func (m *s3API) HeadObject(ctx context.Context, param *s3v2.HeadObjectInput, opts ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	return m.head(ctx, param, opts...)
}

func (m *s3API) GetObject(ctx context.Context, param *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
	return m.get(ctx, param, opts...)
}

func (m *s3API) ListObjectsV2(ctx context.Context, param *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	return m.list(ctx, param, opts...)
}

func (m *s3API) PutObject(ctx context.Context, param *s3v2.PutObjectInput, opts ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
	return m.put(ctx, param, opts...)
}
func (m *s3API) CreateMultipartUpload(ctx context.Context, param *s3v2.CreateMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
	return m.createMPU(ctx, param, opts...)
}

func (m *s3API) UploadPart(ctx context.Context, param *s3v2.UploadPartInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
	return m.putPart(ctx, param, opts...)
}
func (m *s3API) UploadPartCopy(ctx context.Context, param *s3v2.UploadPartCopyInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error) {
	return m.putPartCopy(ctx, param, opts...)
}
func (m *s3API) CompleteMultipartUpload(ctx context.Context, param *s3v2.CompleteMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	return m.completeMPU(ctx, param, opts...)
}

func (m *s3API) AbortMultipartUpload(ctx context.Context, param *s3v2.AbortMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	return m.abortMPU(ctx, param, opts...)
}

func (m *s3API) CopyObject(ctx context.Context, param *s3v2.CopyObjectInput, opts ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
	return m.copy(ctx, param, opts...)
}

func (m *s3API) DeleteObject(ctx context.Context, param *s3v2.DeleteObjectInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	return m.delete(ctx, param, opts...)
}

var _ s3.S3API = (*s3API)(nil)
