# Release notes

Breaking changes since **v0.12.0**.

## Object updates are now `ObjectUpdate`, not `UpdatePlan`

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

## `Stage` is rebuilt around `ObjectUpdate`

- `Stage`'s exported fields (`State`, `DigestAlgorithm`, and the embedded `ContentSource` and `FixitySource`) are gone. Use the `Update()` and `Content()` accessors, plus `ID`, `NextHead`, `DigestAlgorithm`, and `Finalized`.
- `StageBytes`, `StageDir`, `StageFiles`, `Stage.Overlay`, and `Stage.HasContent` are removed. Create a stage with `NewStage(ctx, fsys, dir, id, opts...)`, `Root.NewStage`, or `Object.NewStage`, then add content with `AddFS`, `AddFile`, and `AddBytes`, and change it with `Remove`, `Rename`, and `Clear`. A stage always starts as a draft with an empty `ContentMap`.
- Constructors and `AddFS`/`AddFile` take `StageOption` values (`StageWithDigestAlgorithm`, `StageWithFixity`, `StageWithGoLimit`, `StageWithFilter`, `StageWithHidden`) instead of variadic fixity algorithms.
- `Stage.Finalize(msg, user, opts...)` finalizes the stage's update. The stage's JSON form is unchanged.

## `ContentSource`

`ContentSource` moved to `contentmap.go`. The new `ContentMap` is the standard implementation: it can be saved as JSON, reloaded with `ContentMap.OpenFS` and an `fs.Registry`, and checked with `FastCheck` before `Apply` writes anything. `Object` and `Stage` no longer implement `ContentSource` or `FixitySource`.

## Behavior changes

- `UpdateWithDigestAlgorithm` applies only to new objects and is ignored for existing ones. Converting an existing object's digest algorithm is no longer supported. Loading a saved update whose digest algorithm differs from its base inventory's is an error.
- Objects created in a storage root now default to the root's OCFL spec. Requesting a spec newer than the root's returns `ErrObjectSpecExceedsRoot` (OCFL E081), and `Root.ValidateObject` checks it.
- `Apply` and `Revert` for a new object require its directory to be missing, empty, or to hold only what the update writes. Anything else returns an error wrapping `ErrUpdateConflict`.
- `ObjectUpdate.Add` returns an error for fixity whose value for the update's primary digest disagrees with the digest being added.
