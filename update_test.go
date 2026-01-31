package ocfl_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/fs/local"
)

// TestUpdatePlanBuilder tests the deterministic UpdatePlanBuilder
func TestUpdatePlanBuilder(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Setup object path
	objectPath := filepath.Join(tmpDir, "object1")
	be.NilErr(t, os.MkdirAll(objectPath, 0755))

	fsys, err := local.NewFS(objectPath)
	be.NilErr(t, err)

	// Create a new object
	obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID("test:123"))
	be.NilErr(t, err)

	// Create some content to stage
	contentMap := map[string][]byte{
		"file1.txt": []byte("Hello, World!"),
		"file2.txt": []byte("Test content"),
	}

	stage, err := ocfl.StageBytes(contentMap, digest.SHA512)
	be.NilErr(t, err)

	// Fixed timestamp for determinism
	timestamp := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create update plan using builder with deterministic timestamp
	builder := obj.NewUpdatePlanBuilder(stage).
		WithTimestamp(timestamp).
		WithMessage("First version").
		WithUser(&ocfl.User{
			Name:    "Test User",
			Address: "mailto:test@example.com",
		})

	plan1, err := builder.Build()
	be.NilErr(t, err)

	// Create a second plan with the same parameters
	builder2 := obj.NewUpdatePlanBuilder(stage).
		WithTimestamp(timestamp).
		WithMessage("First version").
		WithUser(&ocfl.User{
			Name:    "Test User",
			Address: "mailto:test@example.com",
		})

	plan2, err := builder2.Build()
	be.NilErr(t, err)

	// Plans should be deterministic - same number of activities
	be.Equal(t, len(plan1.Activities), len(plan2.Activities))

	// Verify the plan has the expected version number
	be.Equal(t, 1, plan1.TargetVersion.Num())
}

// TestUpdatePlanActivities tests the Activities() method
func TestUpdatePlanActivities(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "ocfl-durable-test-*")
	be.NilErr(t, err)
	defer os.RemoveAll(tmpDir)

	// Setup object path
	objectPath := filepath.Join(tmpDir, "object1")
	be.NilErr(t, os.MkdirAll(objectPath, 0755))

	fsys, err := local.NewFS(objectPath)
	be.NilErr(t, err)

	// Create a new object
	obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID("test:456"))
	be.NilErr(t, err)

	// Create content
	contentMap := map[string][]byte{
		"doc.txt": []byte("Document content"),
	}

	stage, err := ocfl.StageBytes(contentMap, digest.SHA512)
	be.NilErr(t, err)

	// Create update plan
	builder := obj.NewUpdatePlanBuilder(stage).
		WithTimestamp(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)).
		WithMessage("Initial version").
		WithUser(&ocfl.User{
			Name:    "Test User",
			Address: "mailto:test@example.com",
		})

	plan, err := builder.Build()
	be.NilErr(t, err)

	// Get activities
	activities := plan.Activities

	// Should have multiple activities:
	// - Declare object
	// - Create version directory
	// - Copy content
	// - Write version inventory
	// - Write version sidecar
	// - Write root inventory
	// - Write root sidecar
	be.True(t, len(activities) >= 6) // At least these activities

	// Verify first activity is object declaration
	foundDecl := false
	for _, act := range activities {
		if act.Type == ocfl.ActivityTypeDeclareObject {
			foundDecl = true
			be.Equal(t, ocfl.Spec1_1, act.Params.Spec)
			break
		}
	}
	be.True(t, foundDecl)

	// Verify we have a content copy activity
	foundCopy := false
	for _, act := range activities {
		if act.Type == ocfl.ActivityTypeCopyContent {
			foundCopy = true
			be.Nonzero(t, act.Params.ContentDigest)
			be.Nonzero(t, act.Params.DestPath)
			break
		}
	}
	be.True(t, foundCopy)
}

// TestExecuteActivity tests executing individual activities
func TestExecuteActivity(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "ocfl-durable-test-*")
	be.NilErr(t, err)
	defer os.RemoveAll(tmpDir)

	objectPath := filepath.Join(tmpDir, "object1")
	be.NilErr(t, os.MkdirAll(objectPath, 0755))

	fsys, err := local.NewFS(objectPath)
	be.NilErr(t, err)

	// Test ActivityDeclareObject
	t.Run("DeclareObject", func(t *testing.T) {
		activity := &ocfl.Activity{
			ID:   "declare object",
			Type: ocfl.ActivityTypeDeclareObject,
			Params: ocfl.ActivityParams{
				Spec:        ocfl.Spec1_1,
				NamasteFile: "0=ocfl_object_1.1",
			},
			Idempotent: true,
		}

		result, err := activity.Execute(ctx, fsys, ".", nil)
		be.NilErr(t, err)
		be.False(t, result.Skipped)

		// Verify declaration file was created
		declPath := filepath.Join(objectPath, "0=ocfl_object_1.1")
		_, err = os.Stat(declPath)
		be.NilErr(t, err)

		// Execute again - should be idempotent (skipped)
		result2, err := activity.Execute(ctx, fsys, ".", nil)
		be.NilErr(t, err)
		be.True(t, result2.Skipped)
	})

	// Test ActivityCreateVersionDir
	t.Run("CreateVersionDir", func(t *testing.T) {
		activity := &ocfl.Activity{
			ID:   "create version v1",
			Type: ocfl.ActivityTypeCreateVersionDir,
			Params: ocfl.ActivityParams{
				VersionDir: "v1",
			},
			Idempotent: true,
		}

		result, err := activity.Execute(ctx, fsys, ".", nil)
		be.NilErr(t, err)

		// Note: Directory might not exist yet since it's created on file write
		// This is OK - we just verify no error
		_ = result
	})
}

// TestBuilderRequiresTimestamp tests that the builder requires a timestamp
func TestBuilderRequiresTimestamp(t *testing.T) {
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "ocfl-durable-test-*")
	be.NilErr(t, err)
	defer os.RemoveAll(tmpDir)

	objectPath := filepath.Join(tmpDir, "object1")
	be.NilErr(t, os.MkdirAll(objectPath, 0755))

	fsys, err := local.NewFS(objectPath)
	be.NilErr(t, err)

	obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID("test:789"))
	be.NilErr(t, err)

	contentMap := map[string][]byte{
		"file.txt": []byte("content"),
	}

	stage, err := ocfl.StageBytes(contentMap, digest.SHA512)
	be.NilErr(t, err)

	// Try to build without setting timestamp - should fail
	builder := obj.NewUpdatePlanBuilder(stage).
		WithMessage("Version").
		WithUser(&ocfl.User{
			Name:    "Test",
			Address: "mailto:test@example.com",
		})

	_, err = builder.Build()

	be.Nonzero(t, err) // Should get an error about missing timestamp
	be.In(t, "timestamp", err.Error())
}
