package ocfl_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
)

func TestUpdatePlan_Marshal(t *testing.T) {
	// create an update
	ctx := context.Background()
	fixture := filepath.Join(`testdata`, `object-fixtures`, `1.0`, `good-objects`)
	fsys, err := local.NewFS(fixture)
	be.NilErr(t, err)
	obj, err := ocfl.NewObject(ctx, fsys, "spec-ex-full")
	be.NilErr(t, err)
	stage, err := ocfl.StageBytes(map[string][]byte{}, digest.SHA512)
	be.NilErr(t, err)
	update, err := obj.NewUpdate(stage, "new version", ocfl.User{Name: "Me"})
	be.NilErr(t, err)
	be.Nonzero(t, len(slices.Collect(update.Steps())))
	bytes, err := update.MarshalBinary()
	be.NilErr(t, err)
	var sameUpdate ocfl.UpdatePlan
	err = sameUpdate.UnmarshalBinary(bytes)
	be.NilErr(t, err)
	be.True(t, update.Eq(&sameUpdate))
}

func TestUpdatePlan_RecoverUpdatePlan(t *testing.T) {
	ctx := context.Background()
	fixture := filepath.Join(`testdata`, `object-fixtures`, `1.0`, `good-objects`, `spec-ex-full`)
	content := filepath.Join(`testdata`, `content-fixture`)
	stagedContent, err := ocfl.StageDir(ctx, ocflfs.DirFS(content), ".", digest.SHA512)
	be.NilErr(t, err)
	errMaxSteps := errors.New("max steps reached")
	// partialUpdate does a partial update and returns the marshalled update
	// value. The update is canceled during the step set by cancelOnStep
	partialUpdate := func(ctx context.Context, objFS *local.FS, cancelOnStep int) ([]byte, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		obj, err := ocfl.NewObject(ctx, objFS, ".", ocfl.ObjectWithID("ark:/12345/bcd987"))
		if err != nil {
			return nil, err
		}
		update, err := obj.NewUpdate(
			stagedContent, "updated version",
			ocfl.User{Name: "Me"},
			ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1),
		)
		if err != nil {
			return nil, err
		}
		if cancelOnStep > len(slices.Collect(update.Steps())) {
			return nil, errMaxSteps
		}
		i := 0
		for step := range update.Steps() {
			if i >= cancelOnStep {
				cancel()
			}
			if err := step.Run(ctx); err != nil {
				break
			}
			i++
		}
		return update.MarshalBinary()
	}
	t.Run("resume update to new object", func(t *testing.T) {
		cancelOnStep := 0
		for {
			objFS, err := local.NewFS(t.TempDir())
			be.NilErr(t, err)
			updateBytes, err := partialUpdate(ctx, objFS, cancelOnStep)
			if errors.Is(err, errMaxSteps) {
				return
			}
			be.NilErr(t, err)
			var resumeUpdate ocfl.UpdatePlan
			err = resumeUpdate.UnmarshalBinary(updateBytes)
			be.NilErr(t, err)
			err = resumeUpdate.BuildSteps(objFS, ".", stagedContent)
			be.NilErr(t, err)
			_, err = resumeUpdate.Apply(ctx)
			be.NilErr(t, err)
			err = ocfl.ValidateObject(ctx, objFS, ".").Err()
			be.NilErr(t, err)
			obj, err := ocfl.NewObject(ctx, objFS, ".")
			be.NilErr(t, err)
			be.Equal(t, ocfl.V(1), obj.Head())
			cancelOnStep++
		}
	})
	t.Run("undo update to new object", func(t *testing.T) {
		cancelOnStep := 0
		for {
			objFS, err := local.NewFS(t.TempDir())
			be.NilErr(t, err)
			updateBytes, err := partialUpdate(ctx, objFS, cancelOnStep)
			if errors.Is(err, errMaxSteps) {
				return
			}
			be.NilErr(t, err)
			var resumeUpdate ocfl.UpdatePlan
			err = resumeUpdate.UnmarshalBinary(updateBytes)
			be.NilErr(t, err)
			err = resumeUpdate.BuildSteps(objFS, ".", stagedContent)
			be.NilErr(t, err)
			switch {
			case resumeUpdate.Completed():
				be.NilErr(t, resumeUpdate.Err())
				//update is complete and can't be reverted
				err = resumeUpdate.Revert(ctx)
				be.Nonzero(t, err)
				// object is valid
				err = ocfl.ValidateObject(ctx, objFS, ".").Err()
				be.NilErr(t, err)
			default:
				// update is incomplete and has errors
				be.Nonzero(t, resumeUpdate.Err())
				err = resumeUpdate.Revert(ctx)
				be.NilErr(t, err)
				// object is empty
				entries, err := ocflfs.ReadDir(ctx, objFS, ".")
				if err != nil {
					be.True(t, errors.Is(err, fs.ErrNotExist))
				}
				be.Zero(t, len(entries))
			}
			cancelOnStep++
		}
	})

	t.Run("resume update for existing object", func(t *testing.T) {
		cancelOnStep := 0
		for {
			objFS, err := local.NewFS(TempDirFixtureCopy(t, fixture))
			be.NilErr(t, err)
			updateBytes, err := partialUpdate(ctx, objFS, cancelOnStep)
			if errors.Is(err, errMaxSteps) {
				return
			}
			be.NilErr(t, err)
			var resumeUpdate ocfl.UpdatePlan
			err = resumeUpdate.UnmarshalBinary(updateBytes)
			be.NilErr(t, err)
			err = resumeUpdate.BuildSteps(objFS, ".", stagedContent)
			be.NilErr(t, err)
			_, err = resumeUpdate.Apply(ctx)
			be.NilErr(t, err)
			err = ocfl.ValidateObject(ctx, objFS, ".").Err()
			be.NilErr(t, err)
			obj, err := ocfl.NewObject(ctx, objFS, ".")
			be.NilErr(t, err)
			be.Equal(t, ocfl.V(4), obj.Head())
			cancelOnStep++
		}
	})

	t.Run("undo update to existing object", func(t *testing.T) {
		cancelOnStep := 0
		for {
			objFS, err := local.NewFS(TempDirFixtureCopy(t, fixture))
			be.NilErr(t, err)
			t.Log("cancel update on step:", cancelOnStep)
			updateBytes, err := partialUpdate(ctx, objFS, cancelOnStep)
			if errors.Is(err, errMaxSteps) {
				return
			}
			be.NilErr(t, err)
			var resumeUpdate ocfl.UpdatePlan
			err = resumeUpdate.UnmarshalBinary(updateBytes)
			be.NilErr(t, err)
			err = resumeUpdate.BuildSteps(objFS, ".", stagedContent)
			be.NilErr(t, err)
			be.NilErr(t, err)
			switch {
			case resumeUpdate.Completed():
				be.NilErr(t, resumeUpdate.Err())
				//update is complete and can't be reverted
				err = resumeUpdate.Revert(ctx)
				be.Nonzero(t, err)
				obj, err := ocfl.NewObject(ctx, objFS, ".")
				be.NilErr(t, err)
				be.Equal(t, ocfl.V(4), obj.Head())
			default:
				// update is incomplete and has errors
				be.Nonzero(t, resumeUpdate.Err())
				err = resumeUpdate.Revert(ctx)
				be.NilErr(t, err)
				obj, err := ocfl.NewObject(ctx, objFS, ".")
				be.NilErr(t, err)
				be.Equal(t, ocfl.V(3), obj.Head())
			}
			// object is valid in any case
			err = ocfl.ValidateObject(ctx, objFS, ".").Err()
			be.NilErr(t, err)
			cancelOnStep++
		}
	})

}
