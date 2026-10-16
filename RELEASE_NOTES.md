# Release notes

Changes since **v0.12.0**. Breaking changes come first, followed by new features and fixes.

## Breaking changes

### Object updates are now `ObjectUpdate`, not `UpdatePlan`

The `UpdatePlan`/`PlanStep` machinery and `InventoryBuilder` are replaced by `ObjectUpdate` (see `update.go`). An update has two phases. As a draft, its new version state is changed with `Add`, `AddAll`, `Remove`, `Rename`, and `Clear`. `Finalize` settles the version metadata and builds the new inventory. The finalized update is then written with `Apply`, or undone with `Revert` if it was interrupted. An update can be saved and loaded as JSON in either phase, and the saved form includes the base inventory, so an interrupted update can be reverted from the saved update alone, or resumed with the saved update and its content source. Instead of tracking the progress of each step, as `UpdatePlan` did, `Apply` and `Revert` decide what to do from the object's root inventory sidecar: a resumed `Apply` runs every step again, and an object that was changed by something else returns an error wrapping `ErrUpdateConflict`.

`Object` is now read-only. Create an update for an object, then apply it.

Removed:

- `UpdatePlan`, `PlanStep`, `PlanSteps` and all their methods
- `Object.NewUpdatePlan`, `Object.ApplyUpdatePlan`, `Object.Update`
- `InventoryBuilder`, `NewInventoryBuilder`, `Object.InventoryBuilder`
- `ObjectUpdateOption` (replaced by `NewUpdateOption`, `FinalizeUpdateOption` and `ApplyUpdateOption`)
- `FixitySource`, `Object.GetContent`, `Object.GetFixity`, `Object.VersionStage`
- `Object.ReadOnly` and `ErrObjectReadOnly`

Replacements:

- `NewObjectUpdate(ctx, fsys, dir, id, opts...)` and `Root.NewObjectUpdate(ctx, id, opts...)` are the only constructors for a new draft `ObjectUpdate` (a saved update can also be loaded from JSON). They read and validate the object's root inventory once and return the update and an `Object` describing that same inventory (for a missing object, the `Object`'s `Exists` returns false). They return an error wrapping `ErrObjectIncomplete` if the object is half-written.
- To update an `Object` you already have, pass `NewUpdateWithExpectedBase(obj.InventoryDigest())` to `NewObjectUpdate`: it returns an error wrapping `ErrUpdateConflict` if the object has changed since it was read. `NewUpdateMustExist` makes `NewObjectUpdate` return an error wrapping `fs.ErrNotExist` for a missing object.
- `ObjectUpdate.Apply(ctx, fsys, dir, src, opts...)` and `ObjectUpdate.Revert` (and `Root.Apply`/`Root.Revert`) write or undo it. `Apply` takes the `ContentSource` for new content and returns the resulting `*Object`. After a successful `Revert`, the update is a draft again, so it can be edited and finalized.
- Update options have a separate type for each step that uses them, so passing an option to a function that doesn't use it is a compile error. The options are renamed to match:
  - `NewUpdateOption`, for `NewObjectUpdate` and `Root.NewObjectUpdate`: `NewUpdateWithDigestAlgorithm`, `NewUpdateWithExpectedBase`, `NewUpdateWithExpectedHead` (formerly `UpdateWithNewHead`), and `NewUpdateMustExist`.
  - `FinalizeUpdateOption`, for `ObjectUpdate.Finalize`: `FinalizeWithVersionCreated`, `FinalizeWithOCFLSpec`, `FinalizeWithUnchangedVersionState`, and `FinalizeWithContentPathFunc` (formerly `UpdateWith…`).
  - `ApplyUpdateOption`, for `Apply` and `Revert` on `ObjectUpdate` and `Root`: `ApplyWithLogger`, `ApplyWithGoLimit` (formerly `UpdateWith…`), and `ApplyWithTrustedContent`. `Revert` ignores `ApplyWithTrustedContent`.
- `NewUpdateWithDigestAlgorithm` sets a new object's digest algorithm (sha512 by default), replacing the stage's `DigestAlgorithm`. For an existing object it must match the object's algorithm, or `NewObjectUpdate` returns an error. As before, an existing object's digest algorithm can't be changed.
- `ErrRevertUpdate` is now `ErrUpdateCompleted`.
- New sentinels: `ErrFinalized`, `ErrNotFinalized`, `ErrMissingContent`, `ErrContentChanged`, `ErrObjectIncomplete`, `ErrUnexpectedHead`, `ErrUpdateConflict`, `ErrObjectSpecExceedsRoot`.

### Staging moves to the new `stage` package

`Stage` is removed. Staging now lives in a new package, `github.com/srerickson/ocfl-go/stage`, built on the exported `ObjectUpdate` API: its functions add files to an `ObjectUpdate` and record their content in a separate content source, and `ObjectUpdate.Apply` and `Root.Apply` take the update and its content source directly. To save an update together with its content, define a type that holds both (see `examples/update`).

- `StageBytes`, `StageDir`, and `StageFiles` are replaced by:
  - `stage.FS(ctx, u, content, fsys, dir, dst, opts...)`
  - `stage.File(ctx, u, content, fsys, name, dst, opts...)`
  - `stage.Bytes(u, content, dst, b, fixity...)`

  Each one digests content with the update's digest algorithm, adds it to `u`, and records the content `u` needs with a `stage.FileStager` or `stage.BytesStager`. Fixity algorithms are set with `stage.WithFixity` instead of variadic arguments (except for `stage.Bytes`).
- The `Stage` type and its methods (`Stage.Overlay`, `Stage.HasContent`) are removed. Use an `ObjectUpdate` for the new version state and a `stage.FileContentMap`, a `stage.BytesContentMap`, or your own `ContentSource` for its content.
- `Object` no longer implements `ContentSource` or `FixitySource`.

### Behavior changes

- `ValidationDigestConcurrency` and `ApplyWithGoLimit` (formerly `UpdateWithGoLimit`) default to `runtime.GOMAXPROCS(0)` instead of `runtime.NumCPU()`. For both, a value less than 1 means the default.
- `stage.Bytes` adds files with hidden names (starting with "."). `StageBytes` skipped them, since it staged its content with `StageDir`.
- Staging digests files concurrently, using `runtime.GOMAXPROCS(0)` goroutines unless `stage.WithGoLimit` sets a different number. `StageDir` and `StageFiles` digested one file at a time.
- Objects created in a storage root now default to the root's OCFL spec. For an update created with `Root.NewObjectUpdate`, `Finalize` returns an error wrapping `ErrObjectSpecExceedsRoot` (OCFL E081) if the requested spec is newer than the root's. `Root.Apply` also refuses updates whose spec is newer than the root's (for example, an update created with `NewObjectUpdate` rather than `Root.NewObjectUpdate`), and `Root.ValidateObject` and `Root.ValidateObjectDir` report it.
- `Apply` and `Revert` for a new object require its directory to be missing, empty, or to hold only what the update writes. Anything else returns an error wrapping `ErrUpdateConflict`. Previously, reverting the first version of an object removed the object directory whatever it held.

## New features

- **Persistable updates.** `ObjectUpdate` implements `json.Marshaler` and `json.Unmarshaler`. `BaseState`, `NewState`, `NextHead`, `BaseInventoryDigest`, and `NewInventoryDigest` describe the update, and `VersionInfo` returns the finalized version's metadata.
- **Content sources.** `stage.FileContentMap` records content as files in FSs and can be saved as JSON. Its FSs are saved with their `MarshalText` form and reopened with `FileContentMap.OpenFS` and an `fs.Registry`. `stage.BytesContentMap` holds content in memory. `ObjectUpdate.NeedsContent` and `ObjectUpdate.NewContentDigests` report which content an update needs, and `ObjectUpdate.AddAll` adds files whose digests are already known.
- **FS registry.** The new `fs.Registry` (`fs.NewRegistry`, `Registry.Append`, `Registry.Open`, `Registry.Schemes`) opens an FS from a configuration string by URL scheme. `Registry.Open` returns an error wrapping `fs.ErrUnknownScheme` for a scheme it doesn't have. `config.Registry` (in `fs/config`) returns a registry for this module's `file`, `s3`, `http`, and `https` backends.
- **Verified content.** `Apply` digests new content as it copies it into the object and returns an error wrapping `ErrContentChanged` if any content doesn't match its digest (for example, because a staged file changed). The mismatched file is removed, and the update can be applied again once the content is restored, or reverted. `ApplyWithTrustedContent` skips verification and copies with `fs.Copy`, so storage backends can copy content without reading it.
- **Detecting interrupted updates.** `NewObject` now wraps its errors for a half-written object (an object declaration without `inventory.json`, or a root inventory sidecar that's missing or doesn't match) in `ErrObjectIncomplete`, as `NewObjectUpdate` does, so callers can tell an object left by an interrupted update from other errors. It returns these errors in the same cases as before.
- **Stage options.** `stage.WithFixity` and `stage.WithGoLimit` apply to `stage.FS` and `stage.File`. `stage.WithFilter` and `stage.WithHidden` change which files `stage.FS` adds: by default it skips hidden files, as `StageDir` did.
- **Examples.** `example_stage_test.go` shows how to build a custom stage type, and `examples/update` shows how to save a stage and resume an interrupted update.

## Fixes

- `NewUpdateWithExpectedHead` (formerly `UpdateWithNewHead`) now enforces the expected version. Previously the option was silently ignored. A mismatch makes `NewObjectUpdate` return an error wrapping `ErrUnexpectedHead`.
- `Apply` checks that its content source has all new content before writing anything. Missing content returns an error wrapping `ErrMissingContent` instead of leaving a partially written object.
- `Apply` and `Revert` stay quiet after the context is canceled.
