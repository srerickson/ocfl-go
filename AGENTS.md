# AGENTS.md

Guidance for AI coding agents (and human contributors) working in this repository.

## What this is

`ocfl-go` is a Go implementation of the [Oxford Common File Layout](https://ocfl.io/)
(OCFL) specification. It's a library, not an application: consumers (like
[ocfl-tools](https://github.com/srerickson/ocfl-tools) and
[ocfl-services](https://github.com/srerickson/ocfl-services)) import it to
create, read, update, and validate OCFL storage roots and objects on local
disk or S3.

The API is under active development and has frequent breaking changes — don't
assume stability across versions when refactoring call sites.

For OCFL domain rules (inventory structure, versioning invariants, storage
layouts, validation codes), see the `ocfl` skill included in this repo
(`.claude/skills/ocfl/SKILL.md`) rather than re-deriving them here. This
document covers the Go module itself: its structure, conventions, and
architecture.

## Module structure

```
ocfl-go/
├── ocfl.go, ocflv1.go      # version dispatch: the `ocfl` interface picks the
│                           # v1.0/v1.1 implementation (ocflV1)
├── root.go                 # Root: an OCFL storage root
├── object.go                # Object: an OCFL object within a root
├── inventory.go              # Inventory / StoredInventory: the JSON inventory model
├── stage.go                  # Stage: a pending logical file tree for a new version
├── update-plan.go            # UpdatePlan: diff between object state and a Stage,
│                             # applied via Object.ApplyUpdatePlan
├── objectstate.go            # ObjectState: parsed contents of an object root directory
├── namaste.go                 # NAMASTE declaration files (0=ocfl_1.1, etc.)
├── vnum.go                   # VNum/VNums: version directory names (v1, v0002, ...) and sequence validation
├── spec.go                   # Spec: OCFL spec version strings
├── validation.go, objectstate.go, validation/  # validation results and error/warning codes
├── digestmap.go               # DigestMap: digest -> paths, the core of inventory manifest/state
│
├── fs/                        # storage backend abstraction (see below)
│   ├── fs.go                  # FS, WriteFS, CopyFS, DirEntriesFS, FileWalker interfaces
│   ├── local/                 # local filesystem backend (atomic writes via temp+rename)
│   ├── s3/                    # AWS S3 backend
│   ├── http/                  # read-only HTTP backend
│   └── internal/imptest/      # shared conformance tests for FS implementations
│
├── extension/                  # OCFL extensions: storage layouts (0002-0012),
│                                # digest algorithms (0001, 0009), the extension registry
├── digest/                     # digest algorithms (sha512, sha256, md5, etc.) and Digester
├── logging/                    # slog helpers
├── internal/
│   ├── pipeline/                # generic fan-out/fan-in worker pool (iter.Seq based)
│   ├── logical-fs/               # io/fs.FS view over an object's logical (versioned) state
│   ├── ocfltest/                  # shared test fixtures/helpers used across packages
│   └── testutil/                  # misc test helpers
├── examples/                   # runnable example programs (listobjects, update, validate)
└── specs/                      # copies of the OCFL spec files used by tests/tools
```

Package boundaries to respect:

- The root package (`ocfl`) holds the version-independent public API plus the
  v1 implementation (`ocflv1.go`). Anything version-specific to a future OCFL
  spec revision should follow the same pattern as `ocflV1`, implementing the
  internal `ocfl` interface in `ocfl.go`.
- `fs` defines storage as a small set of interfaces (`FS`, `WriteFS`,
  `CopyFS`, `DirEntriesFS`, `FileWalker`, `SameBackend`) with package-level
  helper functions (`fs.ReadDir`, `fs.Write`, `fs.WalkFiles`, ...) that type-assert
  against the optional interfaces and fall back to a generic implementation
  when a backend doesn't implement the optimized path. New backends live in
  their own subpackage under `fs/` and should be validated against the shared
  conformance suite in `fs/internal/imptest`.
- `extension` implements the registered OCFL extensions (storage layouts,
  digest algorithms) behind a `Layout`/registry abstraction — adding a new
  registered extension means adding a new `NNNN_*.go` file plus a registry
  entry, not touching core packages.
- `internal/*` packages are implementation details shared across the module;
  they are not part of the public API and can change freely.

## Idiomatic Go conventions used here

- **Go version**: uses recent stdlib features freely — `iter.Seq`/`iter.Seq2`
  range-over-func iterators (see `fs.DirEntries`, `fs.WalkFiles`,
  `pipeline.Results`), generics (`pipeline[Tin, Tout]`), and `slices`/`maps`
  packages instead of hand-rolled loops. Check `go.mod` for the minimum
  version before using newer syntax.
- **Errors**:
  - Sentinel errors are package-level `var Err... = errors.New(...)`, wrapped
    with `fmt.Errorf("...: %w", err)` for context.
  - Prefer wrapping stdlib errors (`fs.ErrNotExist`, `fs.ErrInvalid`,
    `fs.ErrExist`) so callers can use `errors.Is` against familiar sentinels
    instead of package-specific ones, e.g. `ErrObjectNamasteExists =
    fmt.Errorf("...: %w", fs.ErrExist)`.
  - Multiple independent failures (e.g. best-effort `RemoveAll`) are combined
    with `errors.Join`.
- **Interfaces are small and optional-capability based.** Storage backends
  implement the minimal `FS` interface; richer behavior (`WriteFS`, `CopyFS`,
  `DirEntriesFS`, `FileWalker`, `SameBackend`) is detected with a type
  assertion at the call site and falls back to a generic implementation
  otherwise (see `fs.Copy`, `fs.WalkFiles`, `fs.DirEntries`). Follow this
  pattern rather than adding required methods to `FS` itself.
- **Concurrency**: fan-out/fan-in work uses `internal/pipeline`, a generic
  worker pool driven by an `iter.Seq[Tin]` input and returning an
  `iter.Seq[Result[Tin, Tout]]`. Reuse it instead of writing bespoke
  goroutine/channel plumbing for parallel digest/validation work.
- **Atomic writes**: any backend implementing `WriteFS.Write` must make a
  written file appear only in its complete, final form — never a partially
  written file visible mid-write, and a failed/canceled write must leave prior
  contents intact. The local backend does this with temp-file-then-rename in
  the target's own directory; S3 gets it for free from upload semantics. New
  backends must preserve or explicitly document a deviation from this
  guarantee.
- **Doc comments** are written as full sentences directly on exported
  identifiers, `[Type]` / `[pkg.Func]` godoc linking syntax is used
  throughout (e.g. `// like [io/fs.FS.Open]`), and non-obvious contracts
  (ordering guarantees, error semantics, atomicity guarantees) are spelled
  out in the comment rather than left implicit — see `fs.go` for the style to
  match.
- **Testing**:
  - Most tests use the standard `testing` package with `t.Error`/`t.Fatal`.
  - Many newer/table-driven tests use `github.com/carlmjohnson/be` for
    terser assertions: `be.NilErr(t, err)`, `be.Equal(t, want, got)`,
    `be.True(t, cond)`, `be.DeepEqual(t, want, got)`, `be.Nonzero(t, v)`.
    Prefer `be` for new tests unless matching the style of an existing
    file that doesn't use it.
  - Shared FS backend conformance tests live in `fs/internal/imptest` — a new
    storage backend should run against that suite rather than duplicating
    coverage.
  - S3 tests require a running MinIO instance; use `make s3-up` /
    `make s3-test` / `make s3-down`, or `make test-all` to run everything
    including S3. Plain `go test ./...` skips the S3-backed tests.

## Running tests

```sh
go test ./...                    # everything except S3-backed tests
go test ./... -race              # race detector — CI runs this, so should you
                                  # before pushing anything touching fs/,
                                  # internal/pipeline, or other concurrent code
go test ./path/to/pkg/...        # scope to a package while iterating
```

S3-backed tests (`fs/s3/...`) need a MinIO instance:

```sh
make s3-up      # start MinIO via docker-compose
make s3-test    # run fs/s3/... against it (OCFL_TEST_S3=http://localhost:9000)
make s3-down    # stop MinIO
make test-all   # s3-up, full `go test ./...`, s3-down — closest to full CI
```

## Checks to run before committing / on every commit

Mirror what `.github/workflows/go.yml` runs on every push and PR — run these
locally before committing so CI doesn't catch something avoidable:

1. **`go mod tidy`** — then check `git diff go.mod go.sum` is empty. CI fails
   the build if `go mod tidy` would change anything, so run it any time an
   import is added or removed and commit the result.
2. **`gofmt -l .`** (or just save with a `gofmt`-integrated editor/`goimports`)
   — should report no files. Not a separate CI step, but unformatted code is
   an easy, avoidable review nit.
3. **`go vet ./...`** — standard vet checks.
4. **`GOOS=windows go vet ./...`** — the module builds on Windows too (the
   local backend's atomic `Write` relies on `os.Root.Rename` being a
   replacing rename there), so cross-compile vet catches Windows-only
   breakage even though tests only run on Linux in CI.
5. **`go test ./... -count=5 -race`** — CI's exact test invocation.
   `-count=5` reruns each test 5 times to surface flakiness (especially
   relevant for `internal/pipeline` and other concurrent code) and defeats
   Go's test result cache; `-race` catches data races. Add `-run
   TestName -count=5` to repeat-check a single test under development
   instead of the whole suite.
6. If a change touches `fs/`, also run the S3 suite (`make s3-test`) — it's
   not part of the default `go test ./...` run and is easy to forget.

A useful one-liner before pushing:

```sh
go mod tidy && git diff --exit-code go.mod go.sum && \
gofmt -l . && \
go vet ./... && GOOS=windows go vet ./... && \
go test ./... -count=5 -race
```

## Architecture notes

- **Version dispatch**: `ocfl.go` defines an internal `ocfl` interface
  (`Spec`, `ValidateInventory`, `ValidateObjectRoot`, etc.) implemented by
  `ocflV1` in `ocflv1.go` for both OCFL 1.0 and 1.1 (they differ only in a few
  spec-version-sensitive checks). `getOCFL(spec)` resolves a `Spec` to its
  implementation; `latestOCFL()`/`lowestOCFL()` bound the supported range.
  Adding OCFL 2.0 support would mean adding a new implementation of this
  interface, not branching throughout the codebase.
- **Root and Object are thin wrappers over an `fs.FS`.** Neither owns the
  storage; both hold an `fs.FS` + a relative path and re-derive state (layout,
  inventory) by reading from that FS. This is why `NewRoot`/`NewObject` read
  the backend on construction rather than taking pre-parsed state.
- **Object mutation is plan-based, not imperative.** You don't mutate an
  `Object` directly; you build a `Stage` describing the desired logical file
  tree for the next version, diff it against the object's current state to
  get an `UpdatePlan` (`Object.NewUpdatePlan`), and apply that plan
  (`Object.ApplyUpdatePlan`) which performs the actual writes (new version
  content, inventory, sidecar, in the OCFL-mandated order) and updates the
  in-memory `Object` to reflect the result. This separation keeps diff/plan
  logic testable independent of storage I/O.
- **DigestMap is the core inventory data structure.** Both an inventory's
  `manifest` (digest → content paths) and each version's `state` (digest →
  logical paths) are `DigestMap`s. Validation and update logic is built
  around comparing and merging these maps rather than walking file trees
  directly.
- **Validation accumulates rather than fails fast.** `*Validation` /
  `*ObjectValidation` collect errors and warnings (keyed by OCFL spec codes,
  see `validation/code`) while walking a storage root or object, so a single
  validation run reports every problem found instead of stopping at the
  first one. Follow this pattern when extending validation rather than
  returning early on the first error.
- **Extensions are a registry, not a switch statement.** `extension.go` +
  `registry.go` map an extension name (from `ocfl_layout.json` or an
  object/root `extensions/<name>/config.json`) to a Go implementation of the
  `Layout` (or digest algorithm) interface. New registered OCFL extensions
  are added by registering an implementation, not by adding cases to
  existing logic.

## Where to look first

- Public API surface: `ocfl.go`, `root.go`, `object.go`, `stage.go`,
  `update-plan.go`, `inventory.go`.
- Storage abstraction: `fs/fs.go`.
- OCFL domain rules and validation codes: `.claude/skills/ocfl/SKILL.md` and
  its `references/`.
- Worked examples: `examples/listobjects`, `examples/update`,
  `examples/validate`.
