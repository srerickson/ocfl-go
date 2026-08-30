# AGENTS.md

Go implementation of [OCFL](https://ocfl.io/). Library, not an app — breaking
changes are frequent. OCFL domain rules live in the `ocfl` skill
(`.claude/skills/ocfl/SKILL.md`), not here.

## Structure

```
ocfl.go, ocflv1.go   version dispatch (ocfl interface -> ocflV1 for 1.0/1.1)
root.go              Root: storage root
object.go            Object: an object within a root
inventory.go         Inventory / StoredInventory
stage.go             Stage: pending logical tree for next version
update-plan.go       UpdatePlan: Stage diff, applied via Object.ApplyUpdatePlan
objectstate.go       parsed object-root directory contents
namaste.go           NAMASTE declaration files
vnum.go              VNum/VNums: version dir names + sequence validation
spec.go              Spec: OCFL spec version strings
digestmap.go         DigestMap: digest -> paths (manifest/state)
validation.go, validation/   validation results + error/warning codes

fs/                  storage backend abstraction
  fs.go              FS, WriteFS, CopyFS, DirEntriesFS, FileWalker
  local/             local FS backend (atomic write via temp+rename)
  s3/                S3 backend
  http/              read-only HTTP backend
  internal/imptest/  shared backend conformance tests

extension/           OCFL extensions: layouts (0002-0012), digests (0001,0009), registry
digest/              digest algorithms + Digester
logging/             slog helpers
internal/pipeline/   generic fan-out/fan-in worker pool (iter.Seq)
internal/logical-fs/ io/fs.FS view of an object's versioned state
examples/            runnable examples (listobjects, update, validate)
```

Boundaries: root package = version-independent API + v1 impl; new OCFL spec
versions implement the `ocfl` interface (ocfl.go), not branch existing code.
`fs` capabilities (`WriteFS`, `CopyFS`, ...) are optional, detected via type
assertion — don't add required methods to `FS`. New extensions register an
implementation in `extension/`, don't add cases to existing logic.
`internal/*` is not public API.

## Go conventions here

- Use `iter.Seq`/`iter.Seq2` iterators, generics, `slices`/`maps` — not
  hand-rolled loops (see `fs.WalkFiles`, `pipeline[Tin, Tout]`).
- Sentinel errors are package vars wrapping stdlib sentinels
  (`fs.ErrNotExist`, `fs.ErrExist`, ...) so `errors.Is` works for callers.
  Join independent failures with `errors.Join`.
- Interfaces are small and optional-capability based; check with a type
  assertion, fall back to a generic path. Follow this rather than growing
  `FS`.
- Reuse `internal/pipeline` for fan-out/fan-in instead of bespoke
  goroutines/channels.
- `WriteFS.Write` must be atomic: no partial file ever visible, failed write
  leaves prior contents intact. New backends must preserve or document
  otherwise.
- Doc comments: full sentences on exported identifiers, `[Type]`/`[pkg.Func]`
  godoc links, non-obvious contracts spelled out (see `fs.go`).
- Tests: stdlib `testing` in older files; prefer `github.com/carlmjohnson/be`
  (`be.NilErr`, `be.Equal`, `be.True`, `be.DeepEqual`, `be.Nonzero`) for new
  ones. New FS backends must pass `fs/internal/imptest`.

## Tests

```sh
go test ./...              # everything except fs/s3
go test ./... -race
go test ./path/to/pkg/...
```

S3 tests need MinIO:

```sh
make s3-up      # start MinIO
make s3-test    # run fs/s3/... against it
make s3-down
make test-all   # s3-up + go test ./... + s3-down
```

`OCFL_TEST_S3` sets the S3 endpoint used by the S3 tests (default
`http://localhost:$S3_PORT`, `S3_PORT` default `9000`) — override both to
point at a different MinIO/S3 instance.

## Before every commit

CI (`.github/workflows/go.yml`) runs, in order:

```sh
go mod tidy && git diff --exit-code go.mod go.sum   # fails build if dirty
GOOS=windows go vet ./...                           # local backend must build on Windows too
go test ./... -count=5 -race
```

Also run `gofmt -l .` (must be empty) and plain `go vet ./...`. Touching
`fs/`? Run `make s3-test` too — not covered by the default `go test ./...`.

## Architecture

- **Version dispatch**: `ocfl` interface (ocfl.go) implemented by `ocflV1`
  for both 1.0/1.1; `getOCFL(spec)` resolves it.
- **Root/Object are thin `fs.FS` wrappers** — no owned state; re-derived from
  the backend on construction.
- **Mutation is plan-based**: build a `Stage`, diff into an `UpdatePlan`
  (`Object.NewUpdatePlan`), apply it (`Object.ApplyUpdatePlan`) — writes
  content, inventory, sidecar in OCFL-mandated order.
- **DigestMap** (digest -> paths) underlies both manifest and version state;
  validation/update logic compares and merges these, not file trees.
- **Validation accumulates**, never fails fast — `*Validation` /
  `*ObjectValidation` collect every error/warning by spec code
  (`validation/code`) in one pass.
- **Extensions are a registry** (`extension.go`/`registry.go`) mapping a name
  to a `Layout`/digest-algorithm implementation, not a switch statement.

## Where to look first

- API: `ocfl.go`, `root.go`, `object.go`, `stage.go`, `update-plan.go`,
  `inventory.go`.
- Storage: `fs/fs.go`.
- OCFL domain rules: `.claude/skills/ocfl/SKILL.md`.
- Modern Go idioms: `.claude/skills/use-modern-go/SKILL.md`.
- Examples: `examples/listobjects`, `examples/update`, `examples/validate`.
