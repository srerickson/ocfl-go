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
// # Configuration structs
//
// Each entry point's struct carries only the behavior that is legitimately
// implementation-specific. If both backends would pass the same value, the
// knob does not exist and the suite asserts the behavior outright —
// [TestDirEntries] takes no struct for exactly that reason.
//
// # Skip fields
//
// The suite landed before the fixes it exists to guard, so a few assertions
// are not yet satisfied by a backend on main. Those are gated by a Skip*
// field holding the reason, which the caller sets:
//
//	imptest.TestWriteFSRemove(t, fsys, imptest.WriteFSRemove{
//	    RemoveDotIsNotExist:   true,
//	    SkipMissingIsNotExist: "Remove of a missing key returns nil on the s3 backend; see #166",
//	})
//
// A Skip field is a temporary marker for a known defect, not a knob: the PR
// that fixes the defect deletes the line, and its diff then shows exactly
// which behavior it makes good on. A genuine difference between two correct
// implementations gets a knob instead.
//
// # Import direction
//
// Every caller lives in an external test package (package local_test, package
// s3_test). That keeps this package free to grow backend-specific fixtures —
// importing fs/local and fs/s3 — without an import cycle. It is a
// compile-time constraint, not a style preference.
package imptest
