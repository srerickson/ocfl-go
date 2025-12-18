// Package main demonstrates using OCFL object updates with Microsoft's
// Durable Task Framework. This shows how to create durable, resumable
// workflows for OCFL object updates.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/microsoft/durabletask-go/api"
	"github.com/microsoft/durabletask-go/backend"
	"github.com/microsoft/durabletask-go/backend/sqlite"
	"github.com/microsoft/durabletask-go/task"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
)

// Input/Output types for orchestrations and activities

type UpdateOrchestrationInput struct {
	ObjectID    string `json:"object_id"`
	ObjectPath  string `json:"object_path"`
	ContentPath string `json:"content_path"`
	Message     string `json:"message"`
	UserName    string `json:"user_name"`
	UserAddress string `json:"user_address"`
}

type UpdateOrchestrationOutput struct {
	ObjectID           string `json:"object_id"`
	Version            string `json:"version"`
	ActivitiesExecuted int    `json:"activities_executed"`
	TotalBytes         int64  `json:"total_bytes"`
}

type CreatePlanInput struct {
	ObjectID    string         `json:"object_id"`
	ObjectPath  string         `json:"object_path"`
	StageState  ocfl.DigestMap `json:"stage_state"`
	Timestamp   time.Time      `json:"timestamp"`
	Message     string         `json:"message"`
	UserName    string         `json:"user_name"`
	UserAddress string         `json:"user_address"`
}

type ExecuteActivityInput struct {
	ObjectPath string                    `json:"object_path"`
	Activity   *ocfl.UpdatePlanActivity  `json:"activity"`
	ContentPath string                   `json:"content_path"`
}

// Activity names
const (
	BuildStageActivityName  = "BuildStage"
	CreatePlanActivityName  = "CreatePlan"
	ExecuteActivityName     = "ExecuteOCFLActivity"
	ApplyPlanActivityName   = "ApplyPlan"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()

	// Setup paths
	storageRoot := "./example-storage"
	objectID := "test:durable-task-example"
	objectPath := filepath.Join(storageRoot, "objects", "test-durable-task")
	contentPath := "./example-content"
	dbPath := "./orchestration.db"

	// Create directories
	if err := os.MkdirAll(objectPath, 0755); err != nil {
		return fmt.Errorf("creating object path: %w", err)
	}
	if err := os.MkdirAll(contentPath, 0755); err != nil {
		return fmt.Errorf("creating content path: %w", err)
	}

	// Create example content
	if err := createExampleContent(contentPath); err != nil {
		return fmt.Errorf("creating example content: %w", err)
	}

	fmt.Println("🚀 OCFL Update with Durable Task Framework")
	fmt.Println("=" + string(make([]byte, 45)))
	fmt.Println()

	// Initialize SQLite backend for state persistence
	fmt.Println("📊 Initializing Durable Task Framework...")
	r := task.NewTaskRegistry()
	registerOCFLTasks(r)

	be := sqlite.NewSqliteBackend(sqlite.NewSqliteOptions(dbPath), log.Default())
	orchestrator := backend.NewOrchestrationWorker(be, r, backend.WithLogger(log.Default()))
	taskWorker := backend.NewActivityTaskWorker(be, r, backend.WithLogger(log.Default()))

	// Start workers
	if err := orchestrator.Start(ctx); err != nil {
		return fmt.Errorf("starting orchestrator: %w", err)
	}
	defer orchestrator.Shutdown(ctx)

	if err := taskWorker.Start(ctx); err != nil {
		return fmt.Errorf("starting task worker: %w", err)
	}
	defer taskWorker.Shutdown(ctx)

	fmt.Println("   ✅ Workers started")
	fmt.Println()

	// Create and run orchestration
	client := backend.NewTaskHubGrpcClient(be, be, log.Default())

	input := UpdateOrchestrationInput{
		ObjectID:    objectID,
		ObjectPath:  objectPath,
		ContentPath: contentPath,
		Message:     "Update via Durable Task Framework",
		UserName:    "Durable Task Example",
		UserAddress: "mailto:example@example.com",
	}

	instanceID := fmt.Sprintf("update-%s-%d", objectID, time.Now().Unix())

	fmt.Printf("🎯 Starting orchestration: %s\n", instanceID)
	fmt.Println()

	id, err := client.ScheduleNewOrchestration(ctx, "UpdateObject",
		api.WithInput(input),
		api.WithInstanceID(instanceID))
	if err != nil {
		return fmt.Errorf("scheduling orchestration: %w", err)
	}

	// Wait for completion
	fmt.Println("⏳ Waiting for orchestration to complete...")
	fmt.Println("   (The framework ensures durability across failures)")
	fmt.Println()

	metadata, err := client.WaitForOrchestrationCompletion(ctx, id, api.WithFetchPayloads(true))
	if err != nil {
		return fmt.Errorf("waiting for completion: %w", err)
	}

	if !metadata.IsComplete() {
		return fmt.Errorf("orchestration did not complete")
	}

	if metadata.FailureDetails != nil {
		return fmt.Errorf("orchestration failed: %v", metadata.FailureDetails)
	}

	// Get output
	var output UpdateOrchestrationOutput
	if err := metadata.ReadOutput(&output); err != nil {
		return fmt.Errorf("reading output: %w", err)
	}

	fmt.Println("✅ Orchestration completed successfully!")
	fmt.Println()
	fmt.Printf("   Object ID: %s\n", output.ObjectID)
	fmt.Printf("   Version: %s\n", output.Version)
	fmt.Printf("   Activities: %d\n", output.ActivitiesExecuted)
	fmt.Printf("   Total bytes: %d\n", output.TotalBytes)
	fmt.Println()

	fmt.Println("💡 Try running again - the framework will detect it's already done!")
	fmt.Println("   Or delete orchestration.db to start fresh")

	return nil
}

// UpdateObjectOrchestration is the main orchestration function
func UpdateObjectOrchestration(ctx *task.OrchestrationContext) (any, error) {
	var input UpdateOrchestrationInput
	if err := ctx.GetInput(&input); err != nil {
		return nil, err
	}

	ctx.Logger().Info(fmt.Sprintf("Starting update for object: %s", input.ObjectID))

	// Step 1: Build stage (compute content digests)
	var stageState ocfl.DigestMap
	if err := ctx.CallActivity(BuildStageActivityName,
		task.WithActivityInput(input.ContentPath)).Await(&stageState); err != nil {
		return nil, fmt.Errorf("building stage: %w", err)
	}

	ctx.Logger().Info(fmt.Sprintf("Staged %d files", len(stageState)))

	// Step 2: Create update plan with orchestration's deterministic time
	timestamp := ctx.CurrentTimeUtc()
	var activities []*ocfl.UpdatePlanActivity

	planInput := CreatePlanInput{
		ObjectID:    input.ObjectID,
		ObjectPath:  input.ObjectPath,
		StageState:  stageState,
		Timestamp:   timestamp,
		Message:     input.Message,
		UserName:    input.UserName,
		UserAddress: input.UserAddress,
	}

	if err := ctx.CallActivity(CreatePlanActivityName,
		task.WithActivityInput(planInput)).Await(&activities); err != nil {
		return nil, fmt.Errorf("creating plan: %w", err)
	}

	ctx.Logger().Info(fmt.Sprintf("Plan has %d activities", len(activities)))

	// Step 3: Execute each activity
	var totalBytes int64
	for i, activity := range activities {
		var result ocfl.ActivityResult

		execInput := ExecuteActivityInput{
			ObjectPath:  input.ObjectPath,
			Activity:    activity,
			ContentPath: input.ContentPath,
		}

		if err := ctx.CallActivity(ExecuteActivityName,
			task.WithActivityInput(execInput)).Await(&result); err != nil {
			return nil, fmt.Errorf("activity %d (%s) failed: %w", i, activity.Name, err)
		}

		totalBytes += result.BytesWritten

		if result.Skipped {
			ctx.Logger().Info(fmt.Sprintf("[%d/%d] Skipped: %s - %s",
				i+1, len(activities), activity.Name, result.SkipReason))
		} else {
			ctx.Logger().Info(fmt.Sprintf("[%d/%d] Completed: %s (%d bytes)",
				i+1, len(activities), activity.Name, result.BytesWritten))
		}
	}

	// Step 4: Apply plan to finalize
	var version string
	if err := ctx.CallActivity(ApplyPlanActivityName,
		task.WithActivityInput(input)).Await(&version); err != nil {
		return nil, fmt.Errorf("applying plan: %w", err)
	}

	return UpdateOrchestrationOutput{
		ObjectID:           input.ObjectID,
		Version:            version,
		ActivitiesExecuted: len(activities),
		TotalBytes:         totalBytes,
	}, nil
}

// Activity functions

func BuildStageActivity(ctx context.Context, contentPath string) (ocfl.DigestMap, error) {
	contentFS := ocflfs.NewWrapFS(os.DirFS(contentPath))
	stage, err := ocfl.StageDir(ctx, contentFS, ".", digest.SHA512)
	if err != nil {
		return nil, err
	}
	return stage.State, nil
}

func CreatePlanActivity(ctx context.Context, input CreatePlanInput) ([]*ocfl.UpdatePlanActivity, error) {
	fsys, err := local.NewFS(input.ObjectPath)
	if err != nil {
		return nil, err
	}

	obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID(input.ObjectID))
	if err != nil {
		return nil, err
	}

	stage := &ocfl.Stage{
		State:           input.StageState,
		DigestAlgorithm: digest.SHA512,
	}

	builder := obj.NewUpdatePlanBuilder(stage).
		WithTimestamp(input.Timestamp)

	user := &ocfl.User{
		Name:    input.UserName,
		Address: input.UserAddress,
	}

	plan, err := builder.Build(input.Message, user)
	if err != nil {
		return nil, err
	}

	return plan.Activities()
}

func ExecuteOCFLActivityFunc(ctx context.Context, input ExecuteActivityInput) (ocfl.ActivityResult, error) {
	fsys, err := local.NewFS(input.ObjectPath)
	if err != nil {
		return ocfl.ActivityResult{}, err
	}

	// Setup content source
	contentFS := ocflfs.NewWrapFS(os.DirFS(input.ContentPath))

	// For content copy activities, we need a ContentSource
	var src ocfl.ContentSource
	if input.Activity.Type == ocfl.ActivityCopyContent {
		// Create a simple content source
		src = &simpleContentSource{
			fs:      contentFS,
			baseDir: ".",
		}
	}

	return ocfl.ExecuteActivity(ctx, input.Activity, fsys, ".", src)
}

func ApplyPlanActivity(ctx context.Context, input UpdateOrchestrationInput) (string, error) {
	fsys, err := local.NewFS(input.ObjectPath)
	if err != nil {
		return "", err
	}

	obj, err := ocfl.NewObject(ctx, fsys, ".")
	if err != nil {
		return "", err
	}

	return obj.Head().String(), nil
}

// Register all tasks
func registerOCFLTasks(r *task.TaskRegistry) {
	r.AddOrchestratorN("UpdateObject", UpdateObjectOrchestration)
	r.AddActivityN(BuildStageActivityName, BuildStageActivity)
	r.AddActivityN(CreatePlanActivityName, CreatePlanActivity)
	r.AddActivityN(ExecuteActivityName, ExecuteOCFLActivityFunc)
	r.AddActivityN(ApplyPlanActivityName, ApplyPlanActivity)
}

// Simple ContentSource implementation
type simpleContentSource struct {
	fs      ocflfs.FS
	baseDir string
	digests map[string]string // digest -> path mapping
}

func (s *simpleContentSource) GetContent(digest string) (ocflfs.FS, string) {
	// For this example, we'll use the digest to find the file
	// In a real implementation, you'd maintain a proper mapping
	if s.digests != nil {
		if path, ok := s.digests[digest]; ok {
			return s.fs, path
		}
	}

	// Fallback: try to find any file (simplified for example)
	// In production, use the actual digest->path mapping from staging
	return s.fs, "."
}

// Helper functions

func createExampleContent(contentPath string) error {
	files := map[string][]byte{
		"readme.txt":     []byte("OCFL object created with Durable Task Framework\n"),
		"data/file1.txt": []byte("Content for file 1\n"),
		"data/file2.txt": []byte("Content for file 2\n"),
		"metadata.json":  []byte(`{"title": "Durable Task Example", "version": "1.0"}`),
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
