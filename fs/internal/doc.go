// Package internal holds the cross-backend contract test suite for the
// ocflfs interfaces.
//
// Each entry point takes a live backend and asserts the behavior every
// implementation of the interface must share:
//
//	TestWriteFSWriteContract(t, fsys, WriteFSWriteContract{...})
//	TestWriteFSRemoveContract(t, fsys, WriteFSRemoveContract{...})
//	TestWriteFSRemoveAllContract(t, fsys, WriteFSRemoveAllContract{...})
//	TestDirEntriesContract(t, fsys)
//	TestWalkFilesContract(t, fsys, WalkFilesContract{...})
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
// Each contract's struct carries only the parts of the contract that are
// legitimately backend-specific. If both backends would pass the same value,
// the knob does not exist and the suite asserts the behavior outright —
// [TestDirEntriesContract] takes no struct for exactly that reason.
//
// # Skip fields
//
// The suite landed before the fixes it exists to guard, so a few contract
// cases are not yet honored by a backend on main. Those cases are guarded by
// a Skip* field holding the reason, which the caller sets:
//
//	internal.TestWriteFSRemoveContract(t, fsys, internal.WriteFSRemoveContract{
//	    RemoveDotIsNotExist:   true,
//	    SkipMissingIsNotExist: "Remove of a missing key returns nil on the s3 backend; see #166",
//	})
//
// A Skip field is a temporary marker for a known defect, not a knob: the PR
// that fixes the defect deletes the line, and its diff then shows exactly
// which contract it satisfies. A genuine backend difference gets a knob
// instead.
//
// # Import direction
//
// Every caller of a contract lives in an external test package (package
// local_test, package s3_test). That keeps this package free to grow
// backend-specific fixtures — importing fs/local and fs/s3 — without an
// import cycle. It is a compile-time constraint, not a style preference.
package internal
