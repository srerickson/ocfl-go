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
	fn := func(ctx context.Context, param *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
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
	return openFileAPI(fn)
}

type openFileAPI func(context.Context, *s3v2.GetObjectInput, ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error)

func (m openFileAPI) GetObject(ctx context.Context, param *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
	return m(ctx, param, opts...)
}
