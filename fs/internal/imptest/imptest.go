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
// # Options structs
//
// Each entry point's struct carries only what the suite genuinely cannot
// assert for every backend at once. That is a high bar, and most of these
// structs do not clear it: [TestDirEntries] takes no struct at all, and
// [WriteFSWrite] and [WriteFSRemove] carry nothing but Skip fields.
// [WriteFSRemoveAll] holds the only real knobs, because the backends differ
// there in capability rather than in wording — s3 can empty its bucket and
// local cannot remove its own root. The ErrWalk field on [WalkFiles] is a
// fixture rather than a knob: it supplies a backend whose walk fails, and the
// assertions made about that failure are the same either way.
//
// Before adding a knob, check which of the two it is. A field that lets each
// backend name a different error is usually recording a defect, not a
// difference — that is what WriteDotIsError and RemoveDotIsNotExist did, and
// both were deleted in favor of asserting fs.ErrInvalid outright once the
// backends agreed. If both callers would pass the same value, the knob should
// not exist; assert the behavior directly instead.
//
// # Skip fields
//
// The suite landed before the fixes it exists to guard, so a few assertions
// are not yet satisfied by a backend on main. Those are gated by a Skip*
// field holding the reason, which the caller sets:
//
//	imptest.TestWriteFSRemove(t, fsys, imptest.WriteFSRemove{
//	    SkipMissingIsNotExist: "Remove of a missing key returns nil on the s3 backend; see #166",
//	})
//
// A Skip field is a temporary marker for a known defect, not a knob: the PR
// that fixes the defect deletes the line, and its diff then shows exactly
// which behavior it makes good on. Three stand at the moment — local Write
// atomicity (#163), the fileWalk nil-deref (#165), and the s3 Remove above
// (#166).
//
// # Import direction
//
// Every caller lives in an external test package (package local_test, package
// s3_test). That keeps this package free to grow backend-specific fixtures —
// importing fs/local and fs/s3 — without an import cycle. It is a
// compile-time constraint, not a style preference.
package imptest
