package ocfl

import (
	"errors"
	"testing"

	"github.com/carlmjohnson/be"
)

func TestMapDigestConflictErr(t *testing.T) {
	t.Run("creates error with digest", func(t *testing.T) {
		err := &MapDigestConflictErr{Digest: "abc123"}
		be.Nonzero(t, err)
		be.Equal(t, "abc123", err.Digest)
	})

	t.Run("formats error message correctly", func(t *testing.T) {
		err := &MapDigestConflictErr{Digest: "abc123def456"}
		msg := err.Error()
		be.In(t, "digest conflict", msg)
		be.In(t, "abc123def456", msg)
	})

	t.Run("can be detected with errors.As", func(t *testing.T) {
		err := &MapDigestConflictErr{Digest: "test"}
		var target *MapDigestConflictErr
		be.True(t, errors.As(err, &target))
		be.Equal(t, "test", target.Digest)
	})
}

func TestMapPathConflictErr(t *testing.T) {
	t.Run("creates error with path", func(t *testing.T) {
		err := &MapPathConflictErr{Path: "some/path/file.txt"}
		be.Nonzero(t, err)
		be.Equal(t, "some/path/file.txt", err.Path)
	})

	t.Run("formats error message correctly", func(t *testing.T) {
		err := &MapPathConflictErr{Path: "data/document.pdf"}
		msg := err.Error()
		be.In(t, "path conflict", msg)
		be.In(t, "data/document.pdf", msg)
	})

	t.Run("can be detected with errors.As", func(t *testing.T) {
		err := &MapPathConflictErr{Path: "test/path"}
		var target *MapPathConflictErr
		be.True(t, errors.As(err, &target))
		be.Equal(t, "test/path", target.Path)
	})

	t.Run("distinguishes from other error types", func(t *testing.T) {
		err := &MapPathConflictErr{Path: "test"}
		var digestErr *MapDigestConflictErr
		be.False(t, errors.As(err, &digestErr))
	})
}

func TestMapPathInvalidErr(t *testing.T) {
	t.Run("creates error with path", func(t *testing.T) {
		err := &MapPathInvalidErr{Path: "../invalid/path"}
		be.Nonzero(t, err)
		be.Equal(t, "../invalid/path", err.Path)
	})

	t.Run("formats error message correctly", func(t *testing.T) {
		err := &MapPathInvalidErr{Path: "/absolute/path"}
		msg := err.Error()
		be.In(t, "invalid path", msg)
		be.In(t, "/absolute/path", msg)
	})

	t.Run("can be detected with errors.As", func(t *testing.T) {
		err := &MapPathInvalidErr{Path: "bad/path"}
		var target *MapPathInvalidErr
		be.True(t, errors.As(err, &target))
		be.Equal(t, "bad/path", target.Path)
	})

	t.Run("distinguishes from other error types", func(t *testing.T) {
		err := &MapPathInvalidErr{Path: "test"}
		var pathConflict *MapPathConflictErr
		var digestConflict *MapDigestConflictErr
		be.False(t, errors.As(err, &pathConflict))
		be.False(t, errors.As(err, &digestConflict))
	})
}

func TestErrMapMakerExists(t *testing.T) {
	t.Run("is a sentinel error", func(t *testing.T) {
		be.Nonzero(t, ErrMapMakerExists)
		be.Equal(t, "path and digest exist", ErrMapMakerExists.Error())
	})

	t.Run("can be detected with errors.Is", func(t *testing.T) {
		err := ErrMapMakerExists
		be.True(t, errors.Is(err, ErrMapMakerExists))
	})

	t.Run("wrapped error can be detected", func(t *testing.T) {
		wrapped := errors.Join(ErrMapMakerExists, errors.New("additional context"))
		be.True(t, errors.Is(wrapped, ErrMapMakerExists))
	})
}

func TestErrorTypes_Integration(t *testing.T) {
	t.Run("all error types implement error interface", func(t *testing.T) {
		var _ error = &MapDigestConflictErr{}
		var _ error = &MapPathConflictErr{}
		var _ error = &MapPathInvalidErr{}
		var _ error = ErrMapMakerExists
	})

	t.Run("different error instances are distinguishable", func(t *testing.T) {
		errs := []error{
			&MapDigestConflictErr{Digest: "d1"},
			&MapPathConflictErr{Path: "p1"},
			&MapPathInvalidErr{Path: "p2"},
		}

		// Each should only match itself
		var dcErr *MapDigestConflictErr
		var pcErr *MapPathConflictErr
		var piErr *MapPathInvalidErr

		be.True(t, errors.As(errs[0], &dcErr))
		be.False(t, errors.As(errs[0], &pcErr))
		be.False(t, errors.As(errs[0], &piErr))

		be.False(t, errors.As(errs[1], &dcErr))
		be.True(t, errors.As(errs[1], &pcErr))
		be.False(t, errors.As(errs[1], &piErr))

		be.False(t, errors.As(errs[2], &dcErr))
		be.False(t, errors.As(errs[2], &pcErr))
		be.True(t, errors.As(errs[2], &piErr))
	})
}
