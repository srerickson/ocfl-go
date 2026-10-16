# AGENTS.md

Conventions for working in this repo.

## Required arguments: don't nil-check, let it panic

If a function's argument or receiver is required (for example, the
`*ObjectUpdate` passed to `Root.Apply`), do not add `if x == nil`
checks that return an error. Passing nil is a programmer error, not a runtime
condition: let the nil dereference panic, as the standard library does, and say
"x must not be nil" / "x is required" in the doc comment if it isn't obvious.

Do return errors for *invalid values* that callers can plausibly construct at
runtime (e.g. a saved `ObjectUpdate` whose digest algorithm is unsupported), and
for input data problems. Only guard nil where nil is documented as meaningful
(e.g. an optional `ContentSource`, "may be nil").

## Object update synchronization is out of scope

ocfl-go doesn't synchronize or lock object updates: coordinating concurrent
writers to the same object is the caller's job. Use optimistic conflict
detection where possible, but don't add locking, and don't flag its absence as a
bug. Document it instead where it matters.

## Commit messages: no issue tracker IDs

Don't put Linear issue identifiers (e.g. `CRUDE-62`) or other issue-tracker
references in commit messages, either in the subject or the body. Describe
the change itself; link issues from the pull request instead.

## Release flow

`RELEASE_NOTES.md` describes changes since the last release. It should only be
updated as part of the release flow, in a release branch.

Releases are prepared on a `release/vX.Y.Z` branch. In order:

1. Make sure the `Version` constant in `ocfl.go` (`ocfl.Version`) is set to the
   release version, without the `v` prefix (e.g. `"0.13.0"` for `v0.13.0`).
2. Run `go fix ./...` across the entire code base. Commit the result on its
   own, separate from other changes.
3. Update dependencies with `go get -u ./...`, then run `go mod tidy` and
   `go test ./...`. If the `go` directive in `go.mod` changed, update the Go
   version in `.devcontainer/Dockerfile` to match.
4. Replace `RELEASE_NOTES.md` entirely (don't keep old release notes). Cover
   the changes since the previous release tag, with breaking API changes
   called out in their own section.
