// Package main demonstrates using OCFL object updates with a simple
// durable execution pattern. This example shows how to:
// 1. Create an update plan deterministically
// 2. Get the list of activities
// 3. Execute activities individually with retry logic
// 4. Track progress persistently
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
)

// ExecutionState tracks which activities have completed
type ExecutionState struct {
	ObjectID           string                   `json:"object_id"`
	PlanCreatedAt      time.Time                `json:"plan_created_at"`
	CompletedSteps     map[string]bool          `json:"completed_steps"`
	ActivityResults    map[string]ActivityInfo  `json:"activity_results"`
	TotalActivities    int                      `json:"total_activities"`
	CompletedCount     int                      `json:"completed_count"`
}

type ActivityInfo struct {
	Name         string    `json:"name"`
	BytesWritten int64     `json:"bytes_written"`
	Skipped      bool      `json:"skipped"`
	CompletedAt  time.Time `json:"completed_at"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()

	// Setup paths
	storageRoot := "./example-storage"
	objectID := "test:durable-example"
	objectPath := filepath.Join(storageRoot, "objects", "test-durable-example")
	contentPath := "./example-content"
	statePath := "./execution-state.json"

	// Create directories
	if err := os.MkdirAll(objectPath, 0755); err != nil {
		return fmt.Errorf("creating object path: %w", err)
	}
	if err := os.MkdirAll(contentPath, 0755); err != nil {
		return fmt.Errorf("creating content path: %w", err)
	}

	// Create some example content
	if err := createExampleContent(contentPath); err != nil {
		return fmt.Errorf("creating example content: %w", err)
	}

	fmt.Println("🚀 Starting Durable OCFL Object Update Example")
	fmt.Println("=" + string(make([]byte, 48)))

	// Step 1: Load or create execution state
	state, isResume := loadOrCreateState(statePath, objectID)
	if isResume {
		fmt.Printf("\n📂 Resuming execution (completed %d/%d activities)\n",
			state.CompletedCount, state.TotalActivities)
	} else {
		fmt.Println("\n✨ Starting new execution")
	}

	// Step 2: Open OCFL object
	fsys, err := local.NewFS(objectPath)
	if err != nil {
		return fmt.Errorf("creating filesystem: %w", err)
	}

	obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID(objectID))
	if err != nil {
		return fmt.Errorf("creating object: %w", err)
	}

	// Step 3: Stage content
	fmt.Println("\n📦 Staging content...")
	contentFS := ocflfs.NewWrapFS(os.DirFS(contentPath))
	stage, err := ocfl.StageDir(ctx, contentFS, ".", digest.SHA512)
	if err != nil {
		return fmt.Errorf("staging content: %w", err)
	}
	fmt.Printf("   Staged %d files\n", len(stage.State))

	// Step 4: Create update plan (deterministically!)
	fmt.Println("\n📋 Creating update plan...")

	// Use the timestamp from state for determinism (for resume)
	// or create a new one for fresh start
	planTimestamp := state.PlanCreatedAt
	if planTimestamp.IsZero() {
		planTimestamp = time.Now().UTC()
		state.PlanCreatedAt = planTimestamp
	}

	builder := obj.NewUpdatePlanBuilder(stage).
		WithTimestamp(planTimestamp)

	plan, err := builder.Build("Example durable update", &ocfl.User{
		Name:    "Durable Example",
		Address: "mailto:example@example.com",
	})
	if err != nil {
		return fmt.Errorf("building plan: %w", err)
	}

	// Step 5: Get activities
	activities, err := plan.Activities()
	if err != nil {
		return fmt.Errorf("getting activities: %w", err)
	}

	state.TotalActivities = len(activities)
	fmt.Printf("   Plan has %d activities\n", len(activities))

	// Step 6: Execute activities with retry logic
	fmt.Println("\n⚡ Executing activities...")
	fmt.Println("   (Activities are idempotent - safe to retry)")
	fmt.Println()

	for i, activity := range activities {
		// Skip if already completed
		if state.CompletedSteps[activity.Name] {
			fmt.Printf("   [%d/%d] ⏭️  Skipping (already done): %s\n",
				i+1, len(activities), activity.Name)
			continue
		}

		// Execute with retry logic
		var result ocfl.ActivityResult
		var err error

		for attempt := 1; attempt <= 3; attempt++ {
			result, err = executeActivityWithRetry(
				ctx, activity, obj, fsys, stage.ContentSource, attempt,
			)

			if err == nil {
				break
			}

			if attempt < 3 {
				fmt.Printf("      ⚠️  Attempt %d failed, retrying...\n", attempt)
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			}
		}

		if err != nil {
			// Save state before exiting
			if saveErr := saveState(statePath, state); saveErr != nil {
				fmt.Printf("   ⚠️  Failed to save state: %v\n", saveErr)
			}
			return fmt.Errorf("activity %q failed after 3 attempts: %w", activity.Name, err)
		}

		// Mark as completed
		state.CompletedSteps[activity.Name] = true
		state.ActivityResults[activity.Name] = ActivityInfo{
			Name:         activity.Name,
			BytesWritten: result.BytesWritten,
			Skipped:      result.Skipped,
			CompletedAt:  time.Now().UTC(),
		}
		state.CompletedCount++

		// Display progress
		status := "✅"
		if result.Skipped {
			status = "⏭️"
		}
		fmt.Printf("   [%d/%d] %s %s\n", i+1, len(activities), status, activity.Name)
		if result.BytesWritten > 0 {
			fmt.Printf("         📊 %d bytes written\n", result.BytesWritten)
		}
		if result.Skipped {
			fmt.Printf("         💡 %s\n", result.SkipReason)
		}

		// Save state after each activity (durability!)
		if err := saveState(statePath, state); err != nil {
			fmt.Printf("   ⚠️  Warning: Failed to save state: %v\n", err)
		}
	}

	// Step 7: Finalize - apply the plan to update object state
	fmt.Println("\n🎯 Finalizing update...")
	if err := obj.ApplyUpdatePlan(ctx, plan, stage.ContentSource); err != nil {
		return fmt.Errorf("applying update plan: %w", err)
	}

	fmt.Println("\n✅ Update completed successfully!")
	fmt.Printf("   Object ID: %s\n", obj.ID())
	fmt.Printf("   New version: %s\n", obj.Head())
	fmt.Printf("   Total activities: %d\n", state.TotalActivities)
	fmt.Printf("   Bytes written: %d\n", sumBytesWritten(state))

	// Clean up state file on success
	os.Remove(statePath)

	fmt.Println("\n💡 Try running again to see idempotency!")
	fmt.Println("   Or interrupt execution (Ctrl+C) and restart to see resume functionality")

	return nil
}

func executeActivityWithRetry(
	ctx context.Context,
	activity *ocfl.UpdatePlanActivity,
	obj *ocfl.Object,
	fsys *local.FS,
	src ocfl.ContentSource,
	attempt int,
) (ocfl.ActivityResult, error) {
	result, err := ocfl.ExecuteActivity(ctx, activity, fsys, ".", src)
	return result, err
}

func createExampleContent(contentPath string) error {
	files := map[string][]byte{
		"readme.txt":     []byte("This is an example OCFL object created with durable execution.\n"),
		"data/file1.txt": []byte("Content for file 1\n"),
		"data/file2.txt": []byte("Content for file 2\n"),
		"metadata.json":  []byte(`{"title": "Example Object", "version": "1.0"}`),
	}

	for path, content := range files {
		fullPath := filepath.Join(contentPath, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return err
		}
	}

	return nil
}

func loadOrCreateState(path, objectID string) (*ExecutionState, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist - create new state
		return &ExecutionState{
			ObjectID:        objectID,
			CompletedSteps:  make(map[string]bool),
			ActivityResults: make(map[string]ActivityInfo),
		}, false
	}

	var state ExecutionState
	if err := json.Unmarshal(data, &state); err != nil {
		// Corrupted state - start fresh
		return &ExecutionState{
			ObjectID:        objectID,
			CompletedSteps:  make(map[string]bool),
			ActivityResults: make(map[string]ActivityInfo),
		}, false
	}

	return &state, true
}

func saveState(path string, state *ExecutionState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func sumBytesWritten(state *ExecutionState) int64 {
	var total int64
	for _, info := range state.ActivityResults {
		total += info.BytesWritten
	}
	return total
}
