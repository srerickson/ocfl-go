// conformance runs the shared fs implementation test suite (imptest)
// against a real S3 bucket, the same tests the backend's own unit tests run
// against the in-process mock in fs/s3/imptest_test.go.
//
// The bucket must exist and be dedicated to testing: the suite's last
// subtest is RemoveAll("."), which deletes every object in the bucket.
// Bucket-scoped tokens (e.g. Cloudflare R2 temporary credentials) cannot
// create buckets, so bucket creation is deliberately out of scope.
//
// Connection settings come from the standard AWS environment
// (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN, AWS_REGION)
// plus S3_ENDPOINT for the store's endpoint, matching the endpoint env var
// used by the repo's S3 integration tests ($OCFL_TEST_S3). For Cloudflare
// R2:
//
//	export S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
//	export AWS_REGION=auto
//	export AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... AWS_SESSION_TOKEN=...
//
//	go run ./fs/s3/examples/conformance ocfl-test
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"

	// imptest lives under fs/internal, and this program is inside the fs/
	// subtree, so the internal import is legal without any re-vendoring.
	"github.com/srerickson/ocfl-go/fs/internal/imptest"
	"github.com/srerickson/ocfl-go/fs/s3"
)

func main() {
	var endpoint, region, runFilter string
	var listOnly, keep bool
	flag.StringVar(&endpoint, "endpoint", os.Getenv("S3_ENDPOINT"), "S3 endpoint URL (or set S3_ENDPOINT)")
	flag.StringVar(&region, "region", os.Getenv("AWS_REGION"), "AWS region, e.g. \"auto\" for R2 (or set AWS_REGION)")
	flag.StringVar(&runFilter, "run", "", "run only tests whose name matches this regular expression")
	flag.BoolVar(&listOnly, "list", false, "list the tests and exit")
	flag.BoolVar(&keep, "keep", false, "skip the end-of-run bucket cleanup")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] BUCKET\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 && !listOnly {
		flag.Usage()
		os.Exit(2)
	}

	// Each imptest entry point assumes a fresh filesystem — the mock-based
	// suite in fs/s3/imptest_test.go gives every test a brand-new mock
	// bucket, and here that means emptying the shared bucket before each
	// test. In particular TestWalkFiles walks "." and asserts the exact set
	// of files it seeded, so anything left by an earlier test fails it.
	fresh := func(f func(*testing.T)) func(*testing.T) {
		return func(t *testing.T) {
			if err := newFS().RemoveAll(context.Background(), "."); err != nil {
				t.Fatalf("resetting bucket: %v", err)
			}
			f(t)
		}
	}

	tests := []testing.InternalTest{
		{Name: "TestWriteFSWrite", F: fresh(func(t *testing.T) {
			imptest.TestWriteFSWrite(t, newFS())
		})},
		{Name: "TestWriteFSRemove", F: fresh(func(t *testing.T) {
			imptest.TestWriteFSRemove(t, newFS())
		})},
		// S3 has no directories: "." empties the bucket, and a file's key is
		// not under its own prefix, so RemoveAll on a file's own path matches
		// nothing. Same options as the backend's own suite in
		// fs/s3/imptest_test.go.
		{Name: "TestWriteFSRemoveAll", F: fresh(func(t *testing.T) {
			imptest.TestWriteFSRemoveAll(t, newFS(), imptest.WriteFSRemoveAll{
				RemoveAllOnFileRemovesIt: false,
			})
		})},
		{Name: "TestDirEntries", F: fresh(func(t *testing.T) {
			imptest.TestDirEntries(t, newFS())
		})},
		{Name: "TestWalkFiles", F: fresh(func(t *testing.T) {
			imptest.TestWalkFiles(t, newFS(), imptest.WalkFiles{ErrWalk: errWalkFixture})
		})},
	}
	if listOnly {
		for _, tst := range tests {
			fmt.Println(tst.Name)
		}
		return
	}

	bucket := flag.Arg(0)
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		fatal(fmt.Errorf("loading AWS config: %w", err))
	}
	client := s3v2.NewFromConfig(cfg, func(o *s3v2.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		// R2 does not support the flexible checksums the SDK sends by
		// default (aws-sdk-go-v2 2025 default change), and responds
		// BadRequest. Fall back to sending them only when the API requires.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	ctx := context.Background()
	if err := checkAccess(ctx, client, bucket); err != nil {
		fatal(err)
	}
	newFS = func() *s3.BucketFS { return s3.NewBucketFS(client, bucket) }

	// testing.MainStart plus (*testing.M).Run is the supported way to run a
	// []testing.InternalTest from a main package; testing.MainRun's doc says
	// it "should usually still panic" — it exists only for the generated
	// test main package.
	m := testing.MainStart(testDeps{runFilter: runFilter}, tests, nil, nil, nil)
	if code := m.Run(); code != 0 {
		os.Exit(code)
	}
	if !keep {
		// Be a good guest: leave the bucket empty. The suite's last subtest
		// already ran RemoveAll("."), so this matters only when -run skipped
		// it or a test failed mid-way.
		if err := newFS().RemoveAll(ctx, "."); err != nil {
			fatal(fmt.Errorf("cleaning bucket %q: %w", bucket, err))
		}
	}
}

// newFS builds the filesystem under test; it is assigned in main once the
// client exists. Tests never run in parallel here, so a plain variable is
// fine.
var newFS = func() *s3.BucketFS { panic("newFS not initialized") }

// errWalkFixture returns a backend whose walk of "blocked" fails, by
// wrapping the real client in an API that errors ListObjectsV2 for the
// "blocked/" prefix — the same fixture the mock-based suite uses, minus the
// mock.
var errWalkFixture = func(t *testing.T) ocflfs.WriteFS {
	t.Helper()
	return s3.NewBucketFS(&listErrAPI{
		S3API:       newFS().Client(),
		errOnPrefix: "blocked/",
		err:         errors.New("list failed (conformance fixture)"),
	}, newFS().Bucket())
}

// listErrAPI fails ListObjectsV2 for one chosen prefix so the walk-error
// subtest can observe how BucketFS delivers a listing failure.
type listErrAPI struct {
	s3.S3API
	errOnPrefix string
	err         error
}

var _ s3.S3API = (*listErrAPI)(nil)

func (a *listErrAPI) ListObjectsV2(ctx context.Context, in *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	if in.Prefix != nil && *in.Prefix == a.errOnPrefix {
		return nil, a.err
	}
	return a.S3API.ListObjectsV2(ctx, in, opts...)
}

// checkAccess fails fast on a bad bucket, endpoint, or credential instead of
// surfacing the same problem as five test failures.
func checkAccess(ctx context.Context, client *s3v2.Client, bucket string) error {
	_, err := client.ListObjectsV2(ctx, &s3v2.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(1),
	})
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("bucket %q: %s: %s", bucket, apiErr.ErrorCode(), apiErr.ErrorMessage())
	}
	return fmt.Errorf("bucket %q: %w", bucket, err)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "conformance:", err)
	os.Exit(1)
}

// testDeps implements testing's unexported testDeps interface (Go 1.27),
// which testing.MainStart requires. Everything unrelated to name matching is
// a no-op. The fuzz methods' corpusEntry parameter is an alias to an
// anonymous struct in package testing, spelled out here field for field; if
// a Go release changes the shape, this file fails to compile and the stub
// needs updating to match GOROOT/src/testing/fuzz.go.
type testDeps struct{ runFilter string }

func (d testDeps) MatchString(pat, name string) (bool, error) {
	if d.runFilter == "" {
		return true, nil
	}
	// testing passes its -test.run value as pat; conformance applies its own
	// -run flag here instead, so every top-level test reaches tRunner and
	// the filter decides.
	matched, err := regexp.MatchString(d.runFilter, name)
	if err != nil {
		return false, err
	}
	return matched, nil
}

func (testDeps) ImportPath() string {
	return "github.com/srerickson/ocfl-go/fs/s3/examples/conformance"
}
func (testDeps) ModulePath() string              { return "github.com/srerickson/ocfl-go" }
func (testDeps) SetPanicOnExit0(bool)            {}
func (testDeps) StartCPUProfile(io.Writer) error { return nil }
func (testDeps) StopCPUProfile()                 {}
func (testDeps) StartTestLog(io.Writer)          {}
func (testDeps) StopTestLog() error              { return nil }
func (testDeps) ResetCoverage()                  {}
func (testDeps) SnapshotCoverage()               {}
func (testDeps) WriteProfileTo(string, io.Writer, int) error {
	return nil
}
func (testDeps) InitRuntimeCoverage() (string, func(string, string) (string, error), func() float64) {
	return "", nil, nil
}
func (testDeps) CoordinateFuzzing(time.Duration, int64, time.Duration, int64, int, []struct {
	Parent     string
	Path       string
	Data       []byte
	Values     []any
	Generation int
	IsSeed     bool
}, []reflect.Type, string, string) error {
	return errors.New("fuzzing is not supported by conformance")
}
func (testDeps) RunFuzzWorker(func(struct {
	Parent     string
	Path       string
	Data       []byte
	Values     []any
	Generation int
	IsSeed     bool
}) error) error {
	return errors.New("fuzzing is not supported by conformance")
}
func (testDeps) ReadCorpus(string, []reflect.Type) ([]struct {
	Parent     string
	Path       string
	Data       []byte
	Values     []any
	Generation int
	IsSeed     bool
}, error) {
	return nil, errors.New("fuzzing is not supported by conformance")
}
func (testDeps) CheckCorpus([]any, []reflect.Type) error {
	return errors.New("fuzzing is not supported by conformance")
}
