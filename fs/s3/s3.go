// Package s3 implements an OCFL filesystem backend for S3-compatible object
// storage. Each operation the backend supports lives in its own file:
// openfile.go, direntries.go, write.go, copy.go, remove.go, removeall.go and
// walkfiles.go. BucketFS and the API interfaces the operations accept are in
// fs.go; shared helpers are in errors.go and fileinfo.go.
package s3

// megabyte is the unit the multipart part-size constants are expressed in.
const megabyte int64 = 1024 * 1024

var (
	// these are variable because we need pass them as pointers
	delim         = "/"
	maxKeys int32 = 1000
)
