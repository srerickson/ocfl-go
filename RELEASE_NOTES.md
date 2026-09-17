# Draft release notes

Changes since **v0.11.2** (July 17, 2026).

## New functionality

- Added `fs/config` for constructing and round-tripping local, S3, and HTTP(S) backends from URL-style configuration strings. It supports injected clients/loggers and reuses equivalent S3 clients.
- Local writes are now atomic and context-aware, preserve existing file permissions, and prevent reads or writes from escaping the configured root through symlinks. `local.FS` also adds `Close`, `MustNewFS`, and text marshaling.
- Added `fs.SameBackend`; local and S3 backends use it to select native same-backend copies even when represented by separate `FS` values.
- S3 now batches recursive deletes, reports per-key failures, maps missing objects consistently to `fs.ErrNotExist` without discarding the underlying API error, and handles empty buckets and directory-marker objects correctly.
- S3 copies now percent-encode source keys correctly and choose multipart copy from the source size before issuing a copy request. New `ErrNoContentLength` and `ErrIncompleteListing` sentinels make malformed S3 responses detectable with `errors.Is`.
- `WriteFS` behavior is now consistent across backends: `Write(".")` and `Remove(".")` return `fs.ErrInvalid`, while `RemoveAll(".")` empties the storage root without deleting it.

## Breaking API changes

- S3 uploads moved from `feature/s3/manager` to `feature/s3/transfermanager`:
  - `WithUploaderOptions` now accepts `func(*transfermanager.Options)`; uploader option names follow the new API (for example, `PartSizeBytes`).
  - `BucketFS.WriteWithOptions` now accepts `func(*transfermanager.UploadObjectInput)` instead of `func(*s3.PutObjectInput)`.
- Custom S3 clients must update their interfaces: `RemoveAPI` now requires `HeadObject`, and `RemoveAllAPI` requires `DeleteObjects` instead of `DeleteObject`.
- `fs.Copy` calls a backend's native `Copy` only when it also implements `SameBackend` and confirms the source shares that backend. Custom `CopyFS` implementations should add `SameBackend` to retain native-copy dispatch.
- `WriteFS.RemoveAll(".")` is now a backend contract rather than a package-level special case; custom implementations must empty their root themselves. Missing-file removal must return `fs.ErrNotExist`.
- `local.NewFS` now requires an existing directory and opens a root descriptor that callers should close. `local.FS` no longer embeds the exported `DirEntriesFS` field, and absolute or root-escaping symlinks are rejected.
