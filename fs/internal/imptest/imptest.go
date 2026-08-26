// Package imptest is the fs implementation test suite: a set of assertions
// that every implementation of the [ocflfs] interfaces must satisfy, run
// against each backend from that backend's own tests.
//
// Each entry point takes a live backend and exercises one interface method:
//
//	TestWriteFSWrite(t, fsys, WriteFSWrite{...})
//	TestWriteFSRemove(t, fsys, WriteFSRemove{...})
//	TestWriteFSRemoveAll(t, fsys, WriteFSRemoveAll{...})
//	TestDirEntries(t, fsys)
//	TestWalkFiles(t, fsys, WalkFiles{...})
//
// The point is not coverage of the happy path — each backend already has
// that — but the cases where the two families of implementation drift apart.
// A hierarchical backend removes a subtree, a key-value backend removes a key
// prefix; those agree on the obvious case and disagree at the boundary. A
// backend can look correct against its own tests and still be wrong in a way
// that only shows up when a caller swaps local storage for S3.
//
// # File layout
//
// One file per interface under test, so a new assertion has an obvious home:
// writefs.go covers [ocflfs.WriteFS] — Write, Remove and RemoveAll —
// direntriesfs.go covers [ocflfs.DirEntriesFS], and filewalker.go covers
// [ocflfs.FileWalker].
//
// # Options structs
//
// An entry point takes an options struct only when the suite genuinely cannot
// assert something for every backend at once. That is a high bar, and most do
// not clear it: [TestDirEntries], [TestWriteFSWrite] and [TestWriteFSRemove]
// take no struct at all. [WriteFSRemoveAll] holds the only real knob,
// RemoveAllOnFileRemovesIt, because the backends differ there in capability
// rather than in wording: RemoveAll on a file's own path deletes it on a
// hierarchical backend and does not on a key-value one, where a key is not
// under its own prefix. The ErrWalk field on [WalkFiles] is a fixture rather
// than a knob: it supplies a backend whose walk fails, and the assertions
// made about that failure are the same either way.
//
// Before adding a knob, check which of the two it is. A field that lets each
// backend name a different error is usually recording a defect, not a
// difference — that is what WriteDotIsError and RemoveDotIsNotExist did, and
// both were deleted in favor of asserting fs.ErrInvalid outright once the
// backends agreed. If both callers would pass the same value, the knob should
// not exist; assert the behavior directly instead.
//
// # Import direction
//
// Every caller lives in an external test package (package local_test, package
// s3_test). That keeps this package free to grow backend-specific fixtures —
// importing fs/local and fs/s3 — without an import cycle. It is a
// compile-time constraint, not a style preference.
package imptest
