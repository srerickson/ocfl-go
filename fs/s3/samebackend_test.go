package s3_test

import (
	"context"
	"errors"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

func TestBucketFS_SameBackend(t *testing.T) {
	// Two distinct *BucketFS values built from the same client and bucket
	// refer to the same backend even though the FS values differ.
	client := mock.New("any-bucket")
	a := s3.NewBucketFS(client, "same-bucket")
	b := s3.NewBucketFS(client, "same-bucket")
	if a == b {
		t.Fatal("test setup: expected two distinct *BucketFS values")
	}
	if !a.SameBackend(b) {
		t.Error("SameBackend(same client, same bucket) = false, want true")
	}
	if !b.SameBackend(a) {
		t.Error("SameBackend should be symmetric: b.SameBackend(a) = false, want true")
	}

	t.Run("different bucket", func(t *testing.T) {
		client := mock.New("any-bucket")
		a := s3.NewBucketFS(client, "bucket-a")
		b := s3.NewBucketFS(client, "bucket-b")
		if a.SameBackend(b) {
			t.Error("SameBackend(same client, different bucket) = true, want false")
		}
		if b.SameBackend(a) {
			t.Error("SameBackend should be symmetric: b.SameBackend(a) = true, want false")
		}
	})

	t.Run("different client", func(t *testing.T) {
		// Distinct client values (even with identical configuration) are
		// different backends: compare by pointer identity, not by value.
		a := s3.NewBucketFS(mock.New("bucket"), "same-bucket")
		b := s3.NewBucketFS(mock.New("bucket"), "same-bucket")
		if a.SameBackend(b) {
			t.Error("SameBackend(different client, same bucket) = true, want false")
		}
	})

	t.Run("non-BucketFS", func(t *testing.T) {
		client := mock.New("bucket")
		fsys := s3.NewBucketFS(client, "same-bucket")
		if fsys.SameBackend(ocflfs.DirFS(".")) {
			t.Error("SameBackend(non-BucketFS) = true, want false")
		}
	})

	t.Run("same backend self", func(t *testing.T) {
		fsys := s3.NewBucketFS(mock.New("bucket"), "same-bucket")
		if !fsys.SameBackend(fsys) {
			t.Error("SameBackend(self) = false, want true")
		}
	})
}

// TestCopy_SameBackendFastPath verifies that fs.Copy takes the optimized
// CopyFS path when copying between two distinct *BucketFS values that share
// the same client and bucket: the client's CopyObject must be invoked.
// A custom CopyObjectFunc observes the call and aborts with a sentinel error
// so the test can assert which path was taken.
func TestCopy_SameBackendFastPath(t *testing.T) {
	ctx := context.Background()
	client := mock.New("bucket", &mock.Object{
		Key:  "src-file",
		Body: []byte("some content"),
	})
	srcFS := s3.NewBucketFS(client, "bucket")
	dstFS := s3.NewBucketFS(client, "bucket")
	if srcFS == dstFS {
		t.Fatal("test setup: expected two distinct *BucketFS values")
	}
	copyObjectCalled := false
	client.CopyObjectFunc = func(_ context.Context, _ *s3v2.CopyObjectInput, _ ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
		copyObjectCalled = true
		return nil, errors.New("observed optimized copy path")
	}
	_, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
	if !copyObjectCalled {
		t.Fatal("fs.Copy did not call CopyObject: SameBackend fast path was not taken for two same-backend BucketFS values")
	}
	if err == nil {
		t.Fatal("expected the sentinel error from CopyObjectFunc")
	}
}

// TestCopy_DifferentClientSlowPath verifies the fallback: fs.Copy must NOT
// use the CopyFS path when the two BucketFS values have different clients,
// even for the same bucket. CopyObject must never be invoked.
func TestCopy_DifferentClientSlowPath(t *testing.T) {
	ctx := context.Background()
	srcClient := mock.New("bucket", &mock.Object{
		Key:  "src-file",
		Body: []byte("some content"),
	})
	dstClient := mock.New("bucket")
	srcFS := s3.NewBucketFS(srcClient, "bucket")
	dstFS := s3.NewBucketFS(dstClient, "bucket")
	if srcFS.SameBackend(dstFS) {
		t.Fatal("test setup: expected different clients to report different backends")
	}
	srcCopyCalled := false
	dstCopyCalled := false
	srcClient.CopyObjectFunc = func(_ context.Context, _ *s3v2.CopyObjectInput, _ ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
		srcCopyCalled = true
		return nil, errors.New("unexpected CopyObject on src client")
	}
	dstClient.CopyObjectFunc = func(_ context.Context, _ *s3v2.CopyObjectInput, _ ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
		dstCopyCalled = true
		return nil, errors.New("unexpected CopyObject on dst client")
	}
	size, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
	if err != nil {
		t.Fatalf("fallback copy failed: %v", err)
	}
	if size != int64(len("some content")) {
		t.Errorf("copied size = %d, want %d", size, len("some content"))
	}
	if srcCopyCalled || dstCopyCalled {
		t.Error("fs.Copy invoked CopyObject for different clients: SameBackend fast path must not be taken")
	}
}

// nonComparableClient is a value-typed S3API whose struct holds a slice, so
// the type is not comparable: evaluating == on two of these panics at
// runtime. The embedded interface supplies the S3API method set and is never
// called — SameBackend only inspects client identity.
type nonComparableClient struct {
	s3.S3API
	tags []string
}

// comparableClient is a value-typed S3API made only of comparable fields, so
// == is both legal and meaningful for it.
type comparableClient struct {
	s3.S3API
	name string
}

// TestBucketFS_SameBackend_NonComparableClient pins that a non-pointer client
// whose dynamic type is not comparable reports "not the same backend" instead
// of panicking. The kind switch in sameClient does not catch this case — a
// struct is not a pointer-like kind — so the guard has to be an explicit
// Comparable check before ==.
func TestBucketFS_SameBackend_NonComparableClient(t *testing.T) {
	client := nonComparableClient{tags: []string{"x"}}
	a := s3.NewBucketFS(client, "same-bucket")
	b := s3.NewBucketFS(client, "same-bucket")

	// The panic this guards against happens inside SameBackend, so an
	// unguarded implementation fails the test by crashing it rather than by
	// returning a wrong value. Recover to report it as a normal failure.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SameBackend panicked on a non-comparable client: %v", r)
		}
	}()

	// Identity cannot be established for this type, so the answer is false
	// even though both FS values were built from the same client value.
	if a.SameBackend(b) {
		t.Error("SameBackend(non-comparable client) = true, want false (identity is undecidable)")
	}
	if b.SameBackend(a) {
		t.Error("SameBackend should be symmetric for non-comparable clients")
	}
	// Self-comparison goes through the same path: still no panic, still false.
	if a.SameBackend(a) {
		t.Error("SameBackend(self, non-comparable client) = true, want false")
	}
}

// TestBucketFS_SameBackend_ComparableValueClient pins the other half of the
// guard: a non-pointer client that IS comparable must still be compared by
// value, so the Comparable check does not turn every value-typed client into
// a false.
func TestBucketFS_SameBackend_ComparableValueClient(t *testing.T) {
	a := s3.NewBucketFS(comparableClient{name: "one"}, "same-bucket")
	b := s3.NewBucketFS(comparableClient{name: "one"}, "same-bucket")
	if !a.SameBackend(b) {
		t.Error("SameBackend(equal comparable value clients) = false, want true")
	}

	c := s3.NewBucketFS(comparableClient{name: "two"}, "same-bucket")
	if a.SameBackend(c) {
		t.Error("SameBackend(unequal comparable value clients) = true, want false")
	}

	// Different dynamic types never match, comparable or not.
	if a.SameBackend(s3.NewBucketFS(nonComparableClient{}, "same-bucket")) {
		t.Error("SameBackend(different dynamic client types) = true, want false")
	}
}
