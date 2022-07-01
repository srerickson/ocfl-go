package s3fs_test

import (
	"crypto/tls"
	"flag"
	"net/http"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	s3ocfl "github.com/srerickson/ocfl/backend/s3fs"
	"github.com/srerickson/ocfl/backend/test"
)

var (
	endpoint    = flag.String("endpoint", "http://localhost:9000", "s3 endpoint")
	bucket      = flag.String("bucket", "ocfl-test", "bucket name")
	accessKeyID = getenvDefault("TEST_AWS_ACCESS_KEY_ID", "minioadmin")
	secretKey   = getenvDefault("TEST_SECRET_ACCESS_KEY", "minioadmin")
	region      = getenvDefault("S3FS_TEST_AWS_REGION", "us-west-1")
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// Test StoreWriter with S3 Backend
func TestS3Backend(t *testing.T) {
	s3cli := newTestClient(t)
	fsys := s3ocfl.New(s3cli, *bucket)
	test.TestBackend(t, fsys)
}

func newTestClient(t *testing.T) s3iface.S3API {
	t.Helper()
	cl := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	s, err := session.NewSession(
		aws.NewConfig().
			WithEndpoint(*endpoint).
			WithRegion(region).
			WithS3ForcePathStyle(true).
			WithHTTPClient(cl).
			WithCredentials(credentials.NewStaticCredentials(accessKeyID, secretKey, "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return s3.New(s)
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v == "" {
		return def
	}
	return def
}
