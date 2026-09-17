# Draft release notes

Changes since **v0.11.2** (July 17, 2026).

## New functionality

- Added `fs/config` for constructing and round-tripping local, S3, and HTTP(S) backends from URL-style configuration strings (`FSConfig` implements `encoding.TextMarshaler`/`TextUnmarshaler`, so it works as a JSON field). It supports injected S3/HTTP clients and a logger, reuses equivalent S3 clients, and exposes `ResetS3Clients` to drop them.
- All three backends now implement `encoding.TextMarshaler`: `local.FS` renders a `file://` URL, `s3.BucketFS` an `s3://` URL carrying region/endpoint/path-style when its client is an `*s3.Client`, and `http.FS` its base URL. This is what `fs/config` round-trips through.
- Local writes are now atomic and context-aware, preserve existing file permissions, and prevent reads or writes from escaping the configured root through symlinks (every path goes through `os.Root`). `local.FS` also adds `Close` and `MarshalText`, and the package adds `MustNewFS`.
- Added `fs.SameBackend`, an optional interface for reporting that another `FS` refers to the same underlying storage. `fs.Copy` uses it to dispatch a backend's native copy even when source and destination are separate `FS` values: `s3.BucketFS` reports same-bucket/same-client and thereby keeps server-side copies, and `local.FS` reports same absolute root (it has no native `Copy`, so this is for callers and future use).
- S3 now batches recursive deletes (one `DeleteObjects` per listing page), reports per-key failures, maps missing objects consistently to `fs.ErrNotExist` without discarding the underlying API error, and handles empty buckets and directory-marker objects correctly.
- S3 copies now percent-encode source keys correctly and choose multipart copy from the source's HEAD `ContentLength` rather than by attempting a copy and parsing the failure. New `ErrNoContentLength` and `ErrIncompleteListing` sentinels make malformed S3 responses detectable with `errors.Is`.
- `WriteFS` behavior is now consistent across backends and pinned by a shared contract test suite: `Write(".")` and `Remove(".")` return `fs.ErrInvalid`, `Remove` of a missing file returns `fs.ErrNotExist`, and `RemoveAll(".")` empties the storage root without deleting it.
- The partial-listing contract is now documented: a `DirEntriesFS` listing that fails partway yields the entries it read and then the error, and `WalkFiles` may yield files from such a listing before reporting it.
- Added an S3 API conformance example (`fs/s3/example/conformance`) plus a Makefile target and docker-compose file for running the S3 tests against a local S3 implementation.

## Bug fixes

- `runSteps` (object update plans) built its errgroup with `errgroup.WithContext` and then immediately replaced it with a bare `errgroup.Group`, so a failing asynchronous step never canceled its siblings' context. The replacement is gone.
- `fs.WalkFiles`' fallback walk dereferenced a nil `fs.DirEntry` after yielding a listing error, panicking on any error-yielding `DirEntriesFS` — including the error iterator `fs.DirEntries` returns for an `FS` that isn't one.
- `local.FS.DirEntries` reported listing errors with the absolute OS path instead of the name the caller passed in.
- `fs.WrapFS.DirEntries` no longer drops a pending `ReadDir` error when it notices context cancellation first; the two are joined.

## Breaking API changes

- S3 uploads moved from `feature/s3/manager` to `feature/s3/transfermanager` (a pre-1.0 AWS SDK module):
  - `WithUploaderOptions` now accepts `func(*transfermanager.Options)`; uploader option names follow the new API (for example, `PartSizeBytes` rather than `PartSize`).
  - `BucketFS.WriteWithOptions` now accepts `func(*transfermanager.UploadObjectInput)` instead of `func(*s3.PutObjectInput)`.
  - `BucketFS.Write` no longer infers a `ContentLength` from the reader. The transfer manager never forwards that field to a request; it is only a part-sizing hint. A write large enough to need a bigger part size (beyond `PartSizeBytes` × `MaxUploadParts`) must now pass the size explicitly through `WriteWithOptions`.
- Custom S3 clients must update their interfaces: `RemoveAPI` now requires `HeadObject` (for the existence probe that makes `Remove` report `fs.ErrNotExist`), and `RemoveAllAPI` requires `DeleteObjects` instead of `DeleteObject`.
- `fs.Copy` calls a backend's native `Copy` only when it also implements `SameBackend` and confirms the source shares that backend; it no longer compares `srcFS == dstFS`. Custom `CopyFS` implementations should add `SameBackend` to retain native-copy dispatch.
- `WriteFS.RemoveAll(".")` is now a backend contract rather than a package-level special case that enumerated and removed top-level entries; custom implementations must empty their root themselves. `Remove` of a missing file must return `fs.ErrNotExist`, and `Write(".")`/`Remove(".")` must return `fs.ErrInvalid`.
- `local.NewFS` now requires an existing directory and opens a root descriptor that callers should close. `local.FS` no longer embeds the exported `DirEntriesFS` field, and absolute or root-escaping symlinks are rejected even when their target is inside the root.
- S3 `Remove(".")` now returns `fs.ErrInvalid` instead of `fs.ErrNotExist`.

## Dependencies

- AWS SDK v2 packages updated (`aws-sdk-go-v2` v1.42.1 → v1.47.0, `service/s3` v1.104.2 → v1.113.1, `smithy-go` v1.27.3 → v1.28.1), `feature/s3/manager` replaced with `feature/s3/transfermanager` v0.4.7. `golang.org/x/crypto` and `golang.org/x/sync` were updated as well, and the minimum Go version moved from 1.25 to 1.26.
