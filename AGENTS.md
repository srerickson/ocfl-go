# AGENTS.md

Conventions for working in this repository.

## Test file names

**Every test file is named for the single implementation file it covers.**
`write.go` is tested by `write_test.go`. A reader changing a function should
be able to find its tests without grepping, and a reader adding a test should
have exactly one place to put it.

Three rules produce every test file name:

1. **Owner.** The file is named for the implementation file it tests.
2. **Package.** External tests (`package foo_test`) take the plain name.
   Internal tests that reach unexported identifiers take `_internal_test.go`.
3. **Build constraint.** A constraint forces its own file, because one file
   has one `//go:build` line: `_windows_test.go` (implicit GOOS suffix),
   `_unix_test.go` (explicit `//go:build !windows`).

Rules 2 and 3 are language requirements, not preferences — a file cannot hold
two package clauses or two build constraints. **They are the only reasons one
implementation file may own more than one test file.** "These tests feel like
a separate topic" is not one; that is what subtests and section comments are
for.

So `fs/local/write.go` legitimately owns three test files:

```
write.go
  write_test.go            package local_test   — the public contract
  write_internal_test.go   package local        — tempFileName, tempPerm, ...
  write_unix_test.go       package local        — //go:build !windows
```

Prefer the external package. A test that *can* compile as `package foo_test`
belongs there — the compiler decides this, not judgment. Note that the shared
contract callers in `fs/local` and `fs/s3` must be external regardless:
`internal/testutil` imports both backends, so neither can import it back.

Two further variants are allowed:

- `<name>_example_test.go` for `Example` functions (see
  `internal/pipeline/pipeline_example_test.go`).
- `helpers_test.go` for fixtures and assertion helpers used by *several* test
  files in a package. A helper used by one file stays in that file. This
  exists so a shared helper's home does not depend on which test happened to
  need it first — see `fs/s3/helpers_test.go`.

### When an implementation file is a grab-bag

If tests have nowhere sensible to land, that is usually a signal about the
implementation file, not the tests. `fs/s3/s3.go` once held seven unrelated
operations, so no test file could name an owner; splitting it into
`openfile.go`, `write.go`, `copy.go`, `remove.go`, ... made the mapping fall
out naturally. Split the implementation file first, as its own pure-code-motion
commit, then move the tests.

Not every file needs to be split. `fs/fs.go` holds the package's documented API
surface as a coherent unit and keeps one `fs_test.go`.

Some implementation files legitimately have no test file — constants and small
adapters covered through the operations that use them (`fs/s3/s3.go`,
`fs/s3/fileinfo.go`). That is fine. An untested file with real behavior
(`fs/files.go`) is a gap; say so rather than inventing a home for it.

## Test names

`Test<Subject>_<Behavior>`, where `Subject` is the identifier under test:

```
TestOpenFile_MissingKey_Integration
TestS3File_Seek_LogsBodyCloseError
TestFS_Write_OverwriteRegularFile
TestRenameReplace_SymlinkSource
```

- **Name the subject, not the scenario.** `TestSeekWithZip` reads as a test of
  zip files; `TestS3File_Seek_Zip_Integration` reads as a test of `Seek`.
- **Do not name the fixture.** A `_Mock` suffix says how a test is set up, not
  what it asserts, and driving an operation against the mock S3 API is the
  default — so it distinguishes nothing.
- **`_Integration` means gated.** Use it on exactly the tests that gate on
  `testutil.S3Enabled()`, and on no others.
- **Prefer subtests** for behavior variants of one function, ordered so the
  file follows its implementation file's declaration order. Separate concerns
  within a file with a `// --- <concern> ---` comment rather than a new file.

## Shared cross-backend contracts

Behavior that every `ocflfs.WriteFS` backend must share lives in
`internal/testutil` as `<operation>_contract.go`, exporting one entry point
that takes the filesystem under test. Backends call it from the test file that
owns the operation:

```go
// fs/local/remove_test.go
testutil.TestWriteFSRemoveContract(t, fsys, testutil.WriteFSRemoveContract{
    RemoveDotIsNotExist: false,  // the S3 backend passes true
})
```

The struct carries only the parts of the contract that are legitimately
backend-specific. Add a knob when the backends genuinely disagree; if both
return the same thing, drop it and assert directly — a configurable contract
that is never configured differently is just a more confusing contract.
`TestDirEntriesContract` takes no struct for exactly that reason.

## Comments in tests

Write the invariant, not the change history. A comment should say what the code
must do and why, not what a previous implementation did wrong. Keep historical
detail only where it names a hazard a future reader would otherwise walk into.

Never cite line numbers (`foo.go:103-108`) — they are wrong the first time
anything above them changes. Name the function or the test instead.

## Refactoring tests safely

When moving or renaming tests, prove that none were lost. Capture an inventory
of every test *and subtest* before starting, and diff it after:

```sh
go test -count=1 -v ./... 2>&1 \
  | grep -E '^(=== RUN|    +--- )' | sed -E 's/^ *//; s/ \([0-9.]+s\)$//' \
  | sort -u > inv-before.txt
```

A dropped `t.Run` shows up as a deleted line. Pure moves must reproduce the
inventory exactly; a rename commit's diff must match the intended rename map
and nothing else. For a pure code-motion commit, also diff `go doc -all
./<pkg>` before and after.

Keep code motion, test moves, and renames in separate commits — each is
mechanically reviewable on its own, and mixed together none of them are.

## Verification

Before committing:

```sh
gofmt -l .                      # must print nothing
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./fs/...
GOOS=windows go build ./... && GOOS=windows go vet ./fs/local/
```

The Windows check matters: `fs/local` has `_windows`-constrained code that
never compiles otherwise. The S3 integration tests skip unless `OCFL_TEST_S3`
points at an endpoint — if you have one, run `OCFL_TEST_S3=<endpoint> go test
./fs/s3/` before claiming the S3 backend is verified, and say plainly when you
have not.
