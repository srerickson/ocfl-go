package s3

import (
	"reflect"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// Compile-time assertion that *BucketFS implements the optional
// [ocflfs.SameBackend] interface used by [ocflfs.Copy].
var _ ocflfs.SameBackend = (*BucketFS)(nil)

// SameBackend reports whether other refers to the same underlying S3 backend
// as f. It returns true only when other is also a *BucketFS with the same
// bucket name and the same client.
//
// The client is compared by pointer identity: S3 clients are normally pointer
// values (e.g. *s3.Client, *mock.S3API). For comparable, non-pointer client
// types sharing the same dynamic type, interface equality is used as a
// fallback. Client values of different dynamic types never match.
//
// Only the client and bucket are compared. Other BucketFS fields (logger,
// uploader, copy options) do not affect backend identity and are
// intentionally ignored: two distinct *BucketFS values built from the same
// client and bucket refer to the same storage even if they were configured
// differently.
func (f *BucketFS) SameBackend(other ocflfs.FS) bool {
	o, ok := other.(*BucketFS)
	if !ok {
		return false
	}
	if f.bucket != o.bucket {
		return false
	}
	return sameClient(f.client, o.client)
}

// sameClient reports whether two S3API values refer to the same client.
// Interface equality (==) is safe only when the dynamic type is comparable,
// so pointer-like and other non-comparable kinds are compared by pointer
// identity instead. Nil values are identical only to nil.
func sameClient(a, b S3API) bool {
	if a == nil || b == nil {
		return a == b
	}
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	if av.Type() != bv.Type() {
		return false
	}
	switch av.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return av.Pointer() == bv.Pointer()
	default:
		return a == b
	}
}
