package s3_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/backend/s3"
)

func TestWrite(t *testing.T) {
	ctx := context.Background()
	b := newBackend(t)
	_, err := b.ReadDir(ctx, ".")
	be.NilErr(t, err)
	_, err = b.Write(ctx, "test/test3", strings.NewReader("hello"))
	be.NilErr(t, err)
}

func newBackend(t *testing.T) *s3.S3Backend {
	const defaultRegion = "us-east-1"
	ctx := context.Background()
	var opts []func(*config.LoadOptions) error
	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				PartitionID:   "aws",
				URL:           "http://localhost:9000",
				SigningRegion: defaultRegion,
			}, nil
		})
	opts = append(opts, config.WithEndpointResolverWithOptions(customResolver))
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return s3.New(cfg, "test")
}
