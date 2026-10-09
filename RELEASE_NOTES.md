# Release notes

Changes since **v0.12.0**. Breaking changes come first, followed by new features and fixes.

## Breaking changes

### Object updates are now `ObjectUpdate`, not `UpdatePlan`

The `UpdatePlan`/`PlanStep` machinery, `InventoryBuilder`, and the `Stage`-as-input flow are replaced by a persistable `ObjectUpdate` (see `update.go`). An update has a draft phase (`Add`, `Remove`, `Rename`, `Clear`), is settled with `Finalize`, and is written with `Apply` or undone with `Revert`. It can be saved and loaded as JSON (`MarshalJSON`/`UnmarshalJSON`), so an interrupted update can be resumed.

Removed:

- `UpdatePlan`, `PlanStep`, `PlanSteps` and all their methods
- `Object.NewUpdatePlan`, `Object.ApplyUpdatePlan`, `Object.Update`
- `InventoryBuilder`, `NewInventoryBuilder`, `Object.InventoryBuilder`
- `ObjectUpdateOption` (replaced by `UpdateOption`)
- `FixitySource`, `Object.GetContent`, `Object.GetFixity`, `Object.VersionStage`
- `Object.ReadOnly` and `ErrObjectReadOnly`

Replacements:

- `NewUpdate(ctx, fsys, dir, id, opts...)`, `Root.NewUpdate`, and `Object.NewUpdate` create an `ObjectUpdate`.
- `ObjectUpdate.Apply` and `ObjectUpdate.Revert` (and `Root.Apply`/`Root.Revert`) run or undo it. `Apply` returns the resulting `*Object`.
- `UpdateWithNewHead` is now `UpdateWithExpectedHead`. The other `UpdateWith…` options keep their names but return `UpdateOption`.
- `ErrRevertUpdate` is now `ErrUpdateCompleted`.
- `ObjectUpdate.State` is now `NewState` (it pairs with the new `BaseState`).
- New sentinels: `ErrFinalized`, `ErrNotFinalized`, `ErrMissingContent`, `ErrObjectIncomplete`, `ErrUnexpectedHead`, `ErrUpdateConflict`, `ErrObjectSpecExceedsRoot`.

### `Stage` is rebuilt around `ObjectUpdate`

- `Stage`'s exported fields (`State`, `DigestAlgorithm`, and the embedded `ContentSource` and `FixitySource`) are gone. Use the `Update()` and `Content()` accessors, plus `ID`, `NextHead`, `DigestAlgorithm`, and `Finalized`.
- `StageBytes`, `StageDir`, `StageFiles`, `Stage.Overlay`, and `Stage.HasContent` are removed. Create a stage with `NewStage(ctx, fsys, dir, id, opts...)`, `Root.NewStage`, or `Object.NewStage`, then add content with `AddFS`, `AddFile`, and `AddBytes`, and change it with `Remove`, `Rename`, and `Clear`. A stage always starts as a draft with an empty `ContentMap`.
- Constructors and `AddFS`/`AddFile` take `StageOption` values (`StageWithDigestAlgorithm`, `StageWithFixity`, `StageWithGoLimit`, `StageWithFilter`, `StageWithHidden`) instead of variadic fixity algorithms.
- `Stage.Finalize(msg, user, opts...)` finalizes the stage's update. The stage's JSON form is unchanged.

### `ContentSource`

`ContentSource` moved to `contentmap.go`. The new `ContentMap` is the standard implementation: it can be saved as JSON, reloaded with `ContentMap.OpenFS` and an `fs.Registry`, and checked with `FastCheck` before `Apply` writes anything. `Object` and `Stage` no longer implement `ContentSource` or `FixitySource`.

### Behavior changes

- `UpdateWithDigestAlgorithm` applies only to new objects and is ignored for existing ones. Converting an existing object's digest algorithm is no longer supported. Loading a saved update whose digest algorithm differs from its base inventory's is an error.
- Objects created in a storage root now default to the root's OCFL spec. Requesting a spec newer than the root's returns `ErrObjectSpecExceedsRoot` (OCFL E081). `Root.Apply` also refuses updates whose spec is newer than the root's, and `Root.ValidateObject` checks it.
- `Apply` and `Revert` for a new object require its directory to be missing, empty, or to hold only what the update writes. Anything else returns an error wrapping `ErrUpdateConflict`.
- `ObjectUpdate.Add` returns an error for fixity whose value for the update's primary digest disagrees with the digest being added.

## New features

- **Persistable updates.** An `ObjectUpdate` can be saved and loaded as JSON, so an interrupted update can be resumed. `BaseState` exposes the base version's state, and the finalized version's metadata is exposed once the update is finalized.
- **Content tokens.** `fs.ContentToken(info)` returns an opaque string that changes when a file's content changes, or `""` if none is available. File systems can provide their own by implementing the new `fs.ContentTokener` interface.
  - Local files use a token built from their OS stat times (`stat:`).
  - `fs/s3` uses the object's ETag (`etag:`).
  - `fs/http` uses a strong ETag (`etag:`), otherwise Last-Modified in Unix seconds (`lm:`).
- **Detecting changed content.** `ContentMap` saves each file's size and content token. `FastCheck` reports files whose size or token changed since they were added, including same-size edits, so `Apply` can refuse to write a stale file under its old digest. It does not validate digests. Each saved content entry is now `[src, name, size, token]`.
- **Stage options.** `StageWithGoLimit`, `StageWithFilter`, `StageWithHidden`, and `Stage.Clear` give control over how files are added to a stage.

## Fixes

- `UpdateWithExpectedHead` (formerly `UpdateWithNewHead`) now enforces the expected version. Previously the option was silently ignored. A mismatch returns an error wrapping `ErrUnexpectedHead`, and nothing is written.
- Updates validate their inputs before touching storage. A missing digest algorithm or missing content returns an error (wrapping `ErrMissingContent` for missing content) instead of panicking or leaving a partially written object.
- `Apply` and `Revert` stay quiet after the context is canceled.
