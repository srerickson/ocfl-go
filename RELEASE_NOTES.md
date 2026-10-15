# Release notes

Changes since **v0.12.0**. Breaking changes come first, followed by new features and fixes.

## Breaking changes

### Object updates are now `ObjectUpdate`, not `UpdatePlan`

The `UpdatePlan`/`PlanStep` machinery and `InventoryBuilder` are replaced by `ObjectUpdate` (see `update.go`). An update has two phases. As a draft, its new version state is changed with `Add`, `AddAll`, `Remove`, `Rename`, and `Clear`. `Finalize` settles the version metadata and builds the new inventory. The finalized update is then written with `Apply`, or undone with `Revert` if it was interrupted. An update can be saved and loaded as JSON in either phase, and the saved form includes the base inventory, so an interrupted update can be resumed or reverted from the saved update alone.

`Object` is now read-only. Create an update for an object, then apply it.

Removed:

- `UpdatePlan`, `PlanStep`, `PlanSteps` and all their methods
- `Object.NewUpdatePlan`, `Object.ApplyUpdatePlan`, `Object.Update`
- `InventoryBuilder`, `NewInventoryBuilder`, `Object.InventoryBuilder`
- `ObjectUpdateOption` (replaced by `UpdateOption`)
- `FixitySource`, `Object.GetContent`, `Object.GetFixity`, `Object.VersionStage`
- `Object.ReadOnly` and `ErrObjectReadOnly`

Replacements:

- `NewObjectUpdate(ctx, fsys, dir, id, opts...)` and `Root.NewObjectUpdate(ctx, id, opts...)` are the only ways to create a draft `ObjectUpdate`. They read and validate the object's root inventory once and return the update and an `Object` describing that same inventory (for a missing object, the `Object`'s `Exists` returns false). They return an error wrapping `ErrObjectIncomplete` if the object is half-written.
- To update an `Object` you already have, pass `UpdateWithExpectedBase(obj.InventoryDigest())` to `NewObjectUpdate`: it returns an error wrapping `ErrUpdateConflict` if the object has changed since it was read. `UpdateMustExist` makes `NewObjectUpdate` return an error wrapping `fs.ErrNotExist` for a missing object.
- `ObjectUpdate.Apply(ctx, fsys, dir, src, opts...)` and `ObjectUpdate.Revert` (and `Root.Apply`/`Root.Revert`) write or undo it. `Apply` takes the `ContentSource` for new content and returns the resulting `*Object`.
- `UpdateWithNewHead` is now `UpdateWithExpectedHead`. The other `UpdateWith…` options keep their names but return `UpdateOption`.
- `UpdateWithDigestAlgorithm` sets a new object's digest algorithm (sha512 by default), replacing the stage's `DigestAlgorithm`. For an existing object it must match the object's algorithm, or `NewObjectUpdate` returns an error. As before, an existing object's digest algorithm can't be changed.
- `ErrRevertUpdate` is now `ErrUpdateCompleted`.
- New sentinels: `ErrFinalized`, `ErrNotFinalized`, `ErrMissingContent`, `ErrContentChanged`, `ErrObjectIncomplete`, `ErrUnexpectedHead`, `ErrUpdateConflict`, `ErrObjectSpecExceedsRoot`.

### Staging is rebuilt from composable pieces

`Stage` is removed. Staging functions add files to an `ObjectUpdate` and record their content in a separate content source, and `ObjectUpdate.Apply` and `Root.Apply` take the update and its content source directly. To save an update together with its content, define a type that holds both (see `examples/update`).

- `StageBytes`, `StageDir`, and `StageFiles` are replaced by:
  - `StageFS(ctx, u, content, fsys, dir, dst, opts...)`
  - `StageFile(ctx, u, content, fsys, name, dst, opts...)`
  - `StageBytes(u, content, dst, b, fixity...)`

  Each one digests content with the update's digest algorithm, adds it to `u`, and records the content `u` needs with a `FileStager` or `BytesStager`. Fixity algorithms are set with `StageWithFixity` instead of variadic arguments (except for `StageBytes`).
- The `Stage` type and its methods (`Stage.Overlay`, `Stage.HasContent`) are removed. Use an `ObjectUpdate` for the new version state and a `FileContentMap`, a `BytesContentMap`, or your own `ContentSource` for its content.
- `Object` no longer implements `ContentSource` or `FixitySource`.

### Behavior changes

- Objects created in a storage root now default to the root's OCFL spec. Requesting a spec newer than the root's returns `ErrObjectSpecExceedsRoot` (OCFL E081). `Root.Apply` also refuses updates whose spec is newer than the root's (for example, an update created with `NewObjectUpdate` rather than `Root.NewObjectUpdate`), and `Root.ValidateObject` checks it.
- `Apply` and `Revert` for a new object require its directory to be missing, empty, or to hold only what the update writes. Anything else returns an error wrapping `ErrUpdateConflict`.
- `ObjectUpdate.Add` returns an error for fixity whose value for the update's primary digest disagrees with the digest being added.

## New features

- **Persistable updates.** `ObjectUpdate` implements `json.Marshaler` and `json.Unmarshaler`. `BaseState`, `NewState`, `NextHead`, `BaseInventoryDigest`, and `NewInventoryDigest` describe the update, and `VersionInfo` returns the finalized version's metadata.
- **Content sources.** `FileContentMap` records content as files in FSs and can be saved as JSON. Its FSs are saved with their `MarshalText` form and reopened with `FileContentMap.OpenFS` and an `fs.Registry`. `BytesContentMap` holds content in memory. `ObjectUpdate.NeedsContent` and `ObjectUpdate.NewContentDigests` report which content an update needs, and `ObjectUpdate.AddAll` adds files whose digests are already known.
- **FS registry.** The new `fs.Registry` (`fs.NewRegistry`, `Registry.Append`, `Registry.Open`) opens an FS from a configuration string by URL scheme. `config.Registry` returns a registry for this module's `file`, `s3`, `http`, and `https` backends.
- **Verified content.** `Apply` digests new content as it copies it into the object and returns an error wrapping `ErrContentChanged` if any content doesn't match its digest (for example, because a staged file changed). The mismatched file is removed, and the update can be applied again once the content is restored, or reverted. `UpdateWithTrustedContent` skips verification and copies with `fs.Copy`, so storage backends can copy content without reading it.
- **Stage options.** `StageWithFixity`, `StageWithGoLimit`, `StageWithFilter`, and `StageWithHidden` control how `StageFS` and `StageFile` add files.
- **Examples.** `example_stage_test.go` shows how to build a custom stage type, and `examples/update` shows how to save a stage and resume an interrupted update.

## Fixes

- `UpdateWithExpectedHead` (formerly `UpdateWithNewHead`) now enforces the expected version. Previously the option was silently ignored. A mismatch returns an error wrapping `ErrUnexpectedHead`, and nothing is written.
- Updates validate their inputs before touching storage. A missing digest algorithm or missing content returns an error (wrapping `ErrMissingContent` for missing content) instead of panicking or leaving a partially written object.
- `Apply` and `Revert` stay quiet after the context is canceled.
