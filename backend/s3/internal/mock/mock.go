package mock

import (
	"context"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
)

type OpenFileAPI func(context.Context, *s3v2.GetObjectInput, ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error)

func (m OpenFileAPI) GetObject(ctx context.Context, param *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
	return m(ctx, param, opts...)
}
