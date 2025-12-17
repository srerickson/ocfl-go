# OCFL Object Updates with Durable Execution Frameworks

## Executive Summary

This document proposes API changes to enable OCFL object updates within durable execution frameworks, specifically **Temporal** and **Microsoft Durable Task Framework**. The current `UpdatePlan` architecture already has excellent durability primitives (serialization, resumability, reversion), but requires modifications to work naturally with workflow orchestration engines.

## Durable Execution Frameworks Overview

### 1. Temporal

**What it is:** The most popular distributed, scalable durable execution engine for Go. Uses workflow-as-code paradigm with built-in state management, retries, and failure recovery.

**Key Concepts:**
- **Workflows**: Deterministic orchestration logic (must be replay-safe)
- **Activities**: Side-effecting operations (file I/O, API calls, content copying)
- **Signals/Queries**: External communication with running workflows
- **Durable Timers**: Persistent delayed execution
- **Event Sourcing**: All state changes persisted as events

**Workflow Constraints:**
- Must be deterministic (no random, time.Now(), map iteration)
- Cannot do I/O directly (must delegate to Activities)
- Can be replayed from any point in execution history

**Go SDK:** `go.temporal.io/sdk`

### 2. Microsoft Durable Task Framework (durabletask-go)

**What it is:** Lightweight, embeddable orchestration engine designed to run within your process. Simpler than Temporal, fewer operational dependencies.

**Key Concepts:**
- **Orchestrations**: Durable workflow logic (deterministic)
- **Activities**: Task execution units (can have side effects)
- **Replaying**: Similar to Temporal's replay mechanism
- **Embeddable**: Runs in-process via gRPC, no separate cluster needed

**Workflow Constraints:**
- Similar determinism requirements to Temporal
- Must use framework APIs for all state changes
- Activities handle all side effects

**Go SDK:** `github.com/microsoft/durabletask-go`

## Current UpdatePlan Architecture Analysis

### Existing Durability Features

The current implementation **already has** many durable execution concepts:

```go
// UpdatePlan is serializable and resumable
plan, err := obj.NewUpdatePlan(stage, msg, user, opts...)
bytes, _ := plan.MarshalBinary()    // Persist state
plan.UnmarshalBinary(bytes)         // Resume later

// Steps are tracked and reversible
err = obj.ApplyUpdatePlan(ctx, plan, src)
err = plan.Revert(ctx, fsys, path, src)

// Progress tracking
for step := range plan.IncompleteSteps() { ... }
```

### Why it Needs Changes for Durable Execution

**Problem 1: Implicit Persistence**
- Current: Plan must be manually serialized/deserialized
- Frameworks: Automatically persist all state after each step

**Problem 2: Mixed Determinism**
- Current: `NewUpdatePlan()` contains non-deterministic elements (timestamps, digest computation)
- Frameworks: Require clear separation of deterministic orchestration from side-effecting activities

**Problem 3: Step Execution Model**
- Current: Steps executed with internal iterator, async goroutines
- Frameworks: Each activity must be explicitly scheduled through framework APIs

**Problem 4: Context Management**
- Current: Uses standard `context.Context`
- Frameworks: Require workflow-specific contexts (e.g., `workflow.Context` in Temporal)

## Proposed API Changes

### Design Principles

1. **Preserve existing API** for non-durable-execution users
2. **Add new interfaces** optimized for durable execution frameworks
3. **Decompose operations** into deterministic (orchestration) and side-effecting (activities)
4. **Expose step-by-step control** for framework-driven execution

### Core API Additions

#### 1. Activity-Based Update Plan

```go
// UpdatePlanActivity represents a single executable step
type UpdatePlanActivity struct {
    Name          string    // Unique step identifier
    Type          ActivityType
    ContentDigest string    // For content copy activities
    Size          int64     // Expected bytes
    Params        ActivityParams
}

type ActivityType int

const (
    ActivityDeclareObject ActivityType = iota
    ActivityCreateVersionDir
    ActivityCopyContent
    ActivityWriteVersionInventory
    ActivityWriteVersionSidecar
    ActivityWriteRootInventory
    ActivityWriteRootSidecar
)

type ActivityParams struct {
    // Type-specific parameters
    SourcePath      string
    DestPath        string
    InventoryJSON   []byte
    SidecarContent  []byte
}
```

#### 2. Deterministic Plan Builder

```go
// UpdatePlanBuilder creates plans using externally-provided values
// This allows workflows to provide deterministic inputs
type UpdatePlanBuilder struct {
    obj       *Object
    stage     *Stage
    timestamp time.Time  // Provided by workflow
}

// NewUpdatePlanBuilder creates a builder for deterministic plan creation
func (obj *Object) NewUpdatePlanBuilder(stage *Stage) *UpdatePlanBuilder

// WithTimestamp sets creation time (workflow provides this)
func (b *UpdatePlanBuilder) WithTimestamp(t time.Time) *UpdatePlanBuilder

// WithVersionNum explicitly sets version number (for replay safety)
func (b *UpdatePlanBuilder) WithVersionNum(vnum int) *UpdatePlanBuilder

// Build creates plan using provided deterministic values
func (b *UpdatePlanBuilder) Build(msg string, user *User, opts ...UpdateOption) (*UpdatePlan, error)
```

#### 3. Activity Iterator for Framework Integration

```go
// Activities returns all activities in execution order
func (plan *UpdatePlan) Activities() []UpdatePlanActivity

// MarkActivityComplete records activity completion
// Returns updated plan state (for framework to persist)
func (plan *UpdatePlan) MarkActivityComplete(activityName string, result ActivityResult) error

// ActivityByName retrieves activity for execution
func (plan *UpdatePlan) ActivityByName(name string) (*UpdatePlanActivity, error)
```

#### 4. Activity Executor

```go
// ExecuteActivity runs a single activity
// This is what framework activities will call
func ExecuteActivity(ctx context.Context, activity *UpdatePlanActivity, obj *Object, src ContentSource) (ActivityResult, error)

type ActivityResult struct {
    BytesWritten int64
    Error        error
    // Could include digests computed, files created, etc.
}
```

## Framework Integration Examples

### Example 1: Temporal Workflow

```go
package ocfltemporal

import (
    "go.temporal.io/sdk/workflow"
    "github.com/srerickson/ocfl-go"
)

// UpdateObjectWorkflow orchestrates OCFL object updates
func UpdateObjectWorkflow(ctx workflow.Context, params UpdateWorkflowParams) error {
    opts := workflow.ActivityOptions{
        StartToCloseTimeout: time.Hour,
        RetryPolicy: &temporal.RetryPolicy{
            MaximumAttempts: 3,
        },
    }
    ctx = workflow.WithActivityOptions(ctx, opts)

    // Step 1: Build stage (deterministic - just digest map)
    var stageState ocfl.DigestMap
    err := workflow.ExecuteActivity(ctx, BuildStageActivity, params.ContentPath).Get(ctx, &stageState)
    if err != nil {
        return err
    }

    // Step 2: Create update plan (deterministic using workflow time)
    timestamp := workflow.Now(ctx)  // Deterministic time from workflow
    var planActivities []ocfl.UpdatePlanActivity
    err = workflow.ExecuteActivity(ctx, CreateUpdatePlanActivity, CreatePlanParams{
        ObjectPath: params.ObjectPath,
        StageState: stageState,
        Timestamp:  timestamp,
        Message:    params.Message,
        User:       params.User,
    }).Get(ctx, &planActivities)
    if err != nil {
        return err
    }

    // Step 3: Execute each activity
    for _, activity := range planActivities {
        var result ocfl.ActivityResult

        // Use activity-specific options for content copying
        actCtx := ctx
        if activity.Type == ocfl.ActivityCopyContent {
            actCtx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
                StartToCloseTimeout: time.Hour * 24, // Long timeout for large files
                HeartbeatTimeout:    time.Minute,
            })
        }

        err = workflow.ExecuteActivity(actCtx, ExecuteOCFLActivity, ExecuteActivityParams{
            ObjectPath: params.ObjectPath,
            Activity:   activity,
            ContentPath: params.ContentPath,
        }).Get(actCtx, &result)

        if err != nil {
            // Temporal will automatically retry or fail workflow
            return fmt.Errorf("activity %s failed: %w", activity.Name, err)
        }

        // Record progress (will be persisted by Temporal)
        workflow.GetLogger(ctx).Info("Activity completed",
            "name", activity.Name,
            "bytes", result.BytesWritten)
    }

    return nil
}

// BuildStageActivity computes content digests (side effect: reads files)
func BuildStageActivity(ctx context.Context, contentPath string) (ocfl.DigestMap, error) {
    fsys := os.DirFS(contentPath)
    stage, err := ocfl.StageDir(ctx, fsys, ".", ocfl.SHA512)
    if err != nil {
        return nil, err
    }
    return stage.State, nil
}

// CreateUpdatePlanActivity builds the plan (deterministic)
func CreateUpdatePlanActivity(ctx context.Context, params CreatePlanParams) ([]ocfl.UpdatePlanActivity, error) {
    obj, err := ocfl.NewObject(ctx, os.DirFS(params.ObjectPath), ".", ocfl.ObjectWithID(params.ObjectID))
    if err != nil {
        return nil, err
    }

    stage := &ocfl.Stage{
        State:           params.StageState,
        DigestAlgorithm: ocfl.SHA512,
        // ContentSource will be provided during execution
    }

    builder := obj.NewUpdatePlanBuilder(stage).
        WithTimestamp(params.Timestamp).
        WithVersionNum(obj.Inventory().Head.Num() + 1)

    plan, err := builder.Build(params.Message, &params.User)
    if err != nil {
        return nil, err
    }

    return plan.Activities(), nil
}

// ExecuteOCFLActivity runs a single UpdatePlan activity
func ExecuteOCFLActivity(ctx context.Context, params ExecuteActivityParams) (ocfl.ActivityResult, error) {
    activity.RecordHeartbeat(ctx, "starting")

    obj, err := ocfl.NewObject(ctx, os.DirFS(params.ObjectPath), ".")
    if err != nil {
        return ocfl.ActivityResult{}, err
    }

    // Setup content source
    contentFS := os.DirFS(params.ContentPath)
    src := &ocfl.DirContentSource{FS: contentFS}

    // Execute the activity
    result, err := ocfl.ExecuteActivity(ctx, &params.Activity, obj, src)

    // Heartbeat for long-running content copies
    if params.Activity.Type == ocfl.ActivityCopyContent {
        activity.RecordHeartbeat(ctx, result.BytesWritten)
    }

    return result, err
}

type UpdateWorkflowParams struct {
    ObjectPath  string
    ObjectID    string
    ContentPath string
    Message     string
    User        ocfl.User
}

type CreatePlanParams struct {
    ObjectPath string
    ObjectID   string
    StageState ocfl.DigestMap
    Timestamp  time.Time
    Message    string
    User       ocfl.User
}

type ExecuteActivityParams struct {
    ObjectPath  string
    Activity    ocfl.UpdatePlanActivity
    ContentPath string
}
```

**Key Benefits:**
- Temporal handles all state persistence
- Automatic retries on activity failures
- Can pause/resume across worker restarts
- Query workflow progress at any time
- Clear separation of concerns (orchestration vs execution)

### Example 2: Durable Task Framework

```go
package ocfldurable

import (
    "github.com/microsoft/durabletask-go/api"
    "github.com/microsoft/durabletask-go/backend"
    "github.com/srerickson/ocfl-go"
)

// UpdateObjectOrchestration is the durable orchestration function
func UpdateObjectOrchestration(ctx *backend.OrchestrationContext) (any, error) {
    var params UpdateOrchestrationInput
    if err := ctx.GetInput(&params); err != nil {
        return nil, err
    }

    // Step 1: Build stage
    var stageState ocfl.DigestMap
    err := ctx.CallActivity(BuildStageActivityName,
        backend.WithActivityInput(params.ContentPath)).Await(&stageState)
    if err != nil {
        return nil, err
    }

    // Step 2: Create plan with orchestration timestamp
    var activities []ocfl.UpdatePlanActivity
    err = ctx.CallActivity(CreatePlanActivityName, backend.WithActivityInput(CreatePlanInput{
        ObjectPath: params.ObjectPath,
        StageState: stageState,
        Timestamp:  ctx.CurrentTimeUtc, // Deterministic time
        Message:    params.Message,
        User:       params.User,
    })).Await(&activities)
    if err != nil {
        return nil, err
    }

    // Step 3: Execute activities in sequence
    results := make([]ocfl.ActivityResult, 0, len(activities))
    for i, activity := range activities {
        var result ocfl.ActivityResult

        err = ctx.CallActivity(ExecuteActivityName,
            backend.WithActivityInput(ExecuteInput{
                ObjectPath: params.ObjectPath,
                Activity:   activity,
                ContentPath: params.ContentPath,
            }),
        ).Await(&result)

        if err != nil {
            return nil, fmt.Errorf("activity %d (%s) failed: %w", i, activity.Name, err)
        }

        results = append(results, result)
    }

    return UpdateOrchestrationOutput{
        ActivitiesCompleted: len(activities),
        TotalBytesWritten:   sumBytes(results),
    }, nil
}

// Activity implementations
func BuildStageActivity(ctx context.Context, input string) (ocfl.DigestMap, error) {
    fsys := os.DirFS(input)
    stage, err := ocfl.StageDir(ctx, fsys, ".", ocfl.SHA512)
    if err != nil {
        return nil, err
    }
    return stage.State, nil
}

func CreatePlanActivity(ctx context.Context, input CreatePlanInput) ([]ocfl.UpdatePlanActivity, error) {
    obj, err := ocfl.NewObject(ctx, os.DirFS(input.ObjectPath), ".")
    if err != nil {
        return nil, err
    }

    stage := &ocfl.Stage{
        State:           input.StageState,
        DigestAlgorithm: ocfl.SHA512,
    }

    builder := obj.NewUpdatePlanBuilder(stage).
        WithTimestamp(input.Timestamp)

    plan, err := builder.Build(input.Message, &input.User)
    if err != nil {
        return nil, err
    }

    return plan.Activities(), nil
}

func ExecuteOCFLActivityFunc(ctx context.Context, input ExecuteInput) (ocfl.ActivityResult, error) {
    obj, err := ocfl.NewObject(ctx, os.DirFS(input.ObjectPath), ".")
    if err != nil {
        return ocfl.ActivityResult{}, err
    }

    contentFS := os.DirFS(input.ContentPath)
    src := &ocfl.DirContentSource{FS: contentFS}

    return ocfl.ExecuteActivity(ctx, &input.Activity, obj, src)
}

// Registration
func RegisterOCFLOrchestrations(r *backend.TaskRegistry) error {
    r.AddOrchestratorN("UpdateObject", UpdateObjectOrchestration)
    r.AddActivityN(BuildStageActivityName, BuildStageActivity)
    r.AddActivityN(CreatePlanActivityName, CreatePlanActivity)
    r.AddActivityN(ExecuteActivityName, ExecuteOCFLActivityFunc)
    return nil
}

const (
    BuildStageActivityName  = "BuildStage"
    CreatePlanActivityName  = "CreatePlan"
    ExecuteActivityName     = "ExecuteOCFLActivity"
)

type UpdateOrchestrationInput struct {
    ObjectPath  string
    ContentPath string
    Message     string
    User        ocfl.User
}

type CreatePlanInput struct {
    ObjectPath string
    StageState ocfl.DigestMap
    Timestamp  time.Time
    Message    string
    User       ocfl.User
}

type ExecuteInput struct {
    ObjectPath  string
    Activity    ocfl.UpdatePlanActivity
    ContentPath string
}

type UpdateOrchestrationOutput struct {
    ActivitiesCompleted int
    TotalBytesWritten   int64
}

func sumBytes(results []ocfl.ActivityResult) int64 {
    var total int64
    for _, r := range results {
        total += r.BytesWritten
    }
    return total
}
```

## API Migration Path

### Phase 1: Add New APIs (No Breaking Changes)
1. Add `UpdatePlanActivity`, `ActivityType`, `ActivityParams` types
2. Add `UpdatePlanBuilder` with deterministic plan creation
3. Add `Activities()`, `MarkActivityComplete()`, `ActivityByName()` to `UpdatePlan`
4. Add standalone `ExecuteActivity()` function
5. Add example integrations in `examples/temporal/` and `examples/durable-task/`

### Phase 2: Optimize Existing Implementation (Optional)
1. Refactor internal `PlanStep` to use new `UpdatePlanActivity` types
2. Maintain backward compatibility with existing `ApplyUpdatePlan()`

### Phase 3: Documentation
1. Document durable execution patterns
2. Add integration guides for Temporal and Durable Task Framework
3. Provide runnable examples with docker-compose setups

## Benefits of Proposed Changes

### For Temporal Users
- ✅ Workflow replay safety (deterministic plan creation)
- ✅ Automatic state persistence (no manual serialization)
- ✅ Built-in retry logic per activity
- ✅ Queryable workflow state
- ✅ Heartbeats for long-running content copies
- ✅ Distributed execution across workers

### For Durable Task Framework Users
- ✅ Lightweight, embeddable solution
- ✅ No separate cluster needed
- ✅ In-process orchestration
- ✅ Similar durability guarantees
- ✅ Simpler operational model

### For Existing Users
- ✅ No breaking changes to current API
- ✅ Can continue using `Object.Update()` and `ApplyUpdatePlan()`
- ✅ New APIs are opt-in

## Implementation Checklist

- [ ] Define new types: `UpdatePlanActivity`, `ActivityType`, `ActivityParams`, `ActivityResult`
- [ ] Implement `UpdatePlanBuilder` with deterministic inputs
- [ ] Add `Activities()` iterator to `UpdatePlan`
- [ ] Implement `ExecuteActivity()` function
- [ ] Create Temporal integration example with workflow + activities
- [ ] Create Durable Task Framework integration example
- [ ] Write integration tests
- [ ] Document API changes and migration guide
- [ ] Add architectural decision record (ADR)

## Open Questions

1. **Content Source Serialization**: How should `ContentSource` be handled in distributed workflows where activities run on different machines?
   - Option A: Require network-accessible storage (S3, NFS)
   - Option B: Include content digests in workflow state, fetch on-demand in activities
   - Option C: Activities receive content paths, handle access locally

2. **Transaction Boundaries**: Should each activity be a separate transaction, or group related activities?
   - Current proposal: Each activity is independent
   - Alternative: Group inventory writes as single transaction

3. **Idempotency**: How to handle activity retries for non-idempotent operations?
   - Proposal: Use content-addressable storage (digests) for natural idempotency
   - File writes: Check existence before writing

4. **Version Numbering in Distributed Systems**: How to prevent version conflicts when multiple workflows run concurrently?
   - Option A: Workflows acquire locks (supported by both frameworks)
   - Option B: Optimistic concurrency with version detection
   - Option C: Single-writer pattern enforced at workflow level

## References

- [Temporal Go SDK Documentation](https://docs.temporal.io/develop/go)
- [Temporal Go SDK Packages](https://pkg.go.dev/go.temporal.io/sdk)
- [Microsoft Durable Task Framework for Go](https://github.com/microsoft/durabletask-go)
- [The Rise of Durable Execution Engines](https://www.kai-waehner.de/blog/2025/06/05/the-rise-of-the-durable-execution-engine-temporal-restate-in-an-event-driven-architecture-apache-kafka/)
- [OCFL Specification](https://ocfl.io/)
