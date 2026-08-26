// Package s3 implements the [ocflfs] storage interfaces over an S3 bucket, so
// an OCFL storage root can live in a bucket instead of a directory.
//
// [BucketFS] is the implementation. It is constructed over an [S3API] -- the
// full set of SDK methods the package uses -- which the AWS SDK's *s3.Client
// satisfies:
//
//	fsys := s3.NewBucketFS(s3.NewFromConfig(cfg), "my-bucket")
//
// S3API is composed of one narrower interface per operation (OpenFileAPI,
// RemoveAPI, RemoveAllAPI, ...). Those exist so a caller can hand a
// purpose-built client to the package-level helpers, and so this
// documentation can say which SDK calls each operation makes.
//
// # Where S3 differs from a filesystem
//
// The point of [ocflfs] is that a caller can swap backends, so where S3's
// semantics differ from a directory's, this package pays to hide the
// difference rather than passing it on:
//
//   - [BucketFS.Remove] issues a HeadObject before its DeleteObject. Deleting
//     an object is idempotent -- the endpoint answers the same way whether or
//     not the key was there -- and the WriteFS contract requires
//     fs.ErrNotExist for a name that is not there. So Remove costs two round
//     trips, and the pair is not atomic: a key another writer removes in
//     between reads as removed rather than missing.
//
//   - [BucketFS.RemoveAll] lists keys under a prefix and deletes each page
//     with a single DeleteObjects request, so it costs a request per thousand
//     files rather than one per file. Deletion is best-effort across the
//     whole listing, and the per-key failures a DeleteObjects response
//     reports are joined into the returned error rather than being lost
//     behind a successful HTTP status.
//
//   - A missing key is reported as an error satisfying errors.Is(err,
//     fs.ErrNotExist) whatever shape the store's error arrived in -- a typed
//     SDK error, an API error code, or a bare 404 -- with the original error
//     still reachable through errors.As. A missing bucket is deliberately not
//     reported that way: it is a configuration error, not a missing file.
//
// # Compatibility
//
// The interfaces above are part of the API, so changing them breaks any
// implementer that does not embed *s3.Client. Recent changes:
//
//   - RemoveAPI gained HeadObject, for the existence probe described above.
//     S3API already required it through OpenFileAPI, so only a standalone
//     RemoveAPI implementer is affected.
//
//   - RemoveAllAPI now requires DeleteObjects in place of DeleteObject, for
//     the batching described above. An implementer of the whole S3API must
//     add the method; DeleteObject is still required, by RemoveAPI.
package s3
