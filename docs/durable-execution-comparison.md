# API Comparison: Current vs. Durable Execution

This document shows side-by-side comparisons of how object updates work with the current API versus the proposed durable execution-friendly API.

## Current API (Basic Update)

### Simple Update

```go
// Current approach - simple but not framework-integrated
func updateObject(ctx context.Context, obj *ocfl.Object, contentDir string) error {
    // Stage content
    stage, err := ocfl.StageDir(ctx, os.DirFS(contentDir), ".", ocfl.SHA512)
    if err != nil {
        return err
    }

    // Create and apply update in one call
    err = obj.Update(ctx, stage, "Updated content", &ocfl.User{
        Name:    "John Doe",
        Address: "mailto:john@example.com",
    })
    return err
}
```

**Limitations:**
- ❌ No visibility into individual steps
- ❌ Can't distribute work across multiple workers
- ❌ Retries restart entire operation
- ❌ No queryable progress
- ❌ Uses wall-clock time (non-deterministic)

### Resumable Update

```go
// Current approach - manually handle serialization
func updateObjectResumable(ctx context.Context, obj *ocfl.Object, contentDir string, stateFile string) error {
    stage, err := ocfl.StageDir(ctx, os.DirFS(contentDir), ".", ocfl.SHA512)
    if err != nil {
        return err
    }

    var plan *ocfl.UpdatePlan

    // Try to resume existing plan
    if data, err := os.ReadFile(stateFile); err == nil {
        plan = &ocfl.UpdatePlan{}
        if err := plan.UnmarshalBinary(data); err != nil {
            return fmt.Errorf("unmarshaling plan: %w", err)
        }
    } else {
        // Create new plan
        plan, err = obj.NewUpdatePlan(stage, "Updated content", &ocfl.User{
            Name:    "John Doe",
            Address: "mailto:john@example.com",
        })
        if err != nil {
            return err
        }
    }

    // Save plan state before applying
    data, err := plan.MarshalBinary()
    if err != nil {
        return err
    }
    if err := os.WriteFile(stateFile, data, 0644); err != nil {
        return err
    }

    // Apply plan
    err = obj.ApplyUpdatePlan(ctx, plan, stage.ContentSource)
    if err != nil {
        // Plan state is saved - could resume later
        return err
    }

    // Clean up state file on success
    os.Remove(stateFile)
    return nil
}
```

**Limitations:**
- ⚠️ Manual state management (save/load)
- ⚠️ No automatic retries
- ⚠️ Progress tracking requires custom code
- ⚠️ No distributed execution

---

## Proposed API (Durable Execution)

### Temporal Workflow

```go
// Proposed approach - full workflow orchestration
func UpdateObjectWorkflow(ctx workflow.Context, params UpdateParams) error {
    ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
        StartToCloseTimeout: time.Hour,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    5,
        },
    })

    // Step 1: Build stage (run as activity for idempotency)
    var stageState ocfl.DigestMap
    err := workflow.ExecuteActivity(ctx, BuildStageActivity, params.ContentPath).
        Get(ctx, &stageState)
    if err != nil {
        return err
    }

    // Step 2: Create plan deterministically
    timestamp := workflow.Now(ctx) // Deterministic time!
    var activities []ocfl.UpdatePlanActivity
    err = workflow.ExecuteActivity(ctx, CreatePlanActivity, CreatePlanParams{
        ObjectPath: params.ObjectPath,
        StageState: stageState,
        Timestamp:  timestamp,
        Message:    params.Message,
        User:       params.User,
    }).Get(ctx, &activities)
    if err != nil {
        return err
    }

    // Step 3: Execute each activity with progress tracking
    for i, activity := range activities {
        var result ocfl.ActivityResult

        // Custom options for large file transfers
        actCtx := ctx
        if activity.Type == ocfl.ActivityCopyContent && activity.Size > 100*1024*1024 {
            actCtx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
                StartToCloseTimeout: 24 * time.Hour,
                HeartbeatTimeout:    time.Minute,
            })
        }

        err = workflow.ExecuteActivity(actCtx, ExecuteOCFLActivity, ExecuteActivityParams{
            ObjectPath: params.ObjectPath,
            Activity:   activity,
            ContentPath: params.ContentPath,
        }).Get(actCtx, &result)

        if err != nil {
            return fmt.Errorf("step %d/%d (%s) failed: %w",
                i+1, len(activities), activity.Name, err)
        }

        workflow.GetLogger(ctx).Info("Progress",
            "step", i+1,
            "total", len(activities),
            "activity", activity.Name,
            "bytes", result.BytesWritten)
    }

    return nil
}

// Activities are simple wrappers
func BuildStageActivity(ctx context.Context, contentPath string) (ocfl.DigestMap, error) {
    stage, err := ocfl.StageDir(ctx, os.DirFS(contentPath), ".", ocfl.SHA512)
    if err != nil {
        return nil, err
    }
    return stage.State, nil
}

func CreatePlanActivity(ctx context.Context, params CreatePlanParams) ([]ocfl.UpdatePlanActivity, error) {
    obj, err := ocfl.NewObject(ctx, os.DirFS(params.ObjectPath), ".")
    if err != nil {
        return nil, err
    }

    stage := &ocfl.Stage{State: params.StageState, DigestAlgorithm: ocfl.SHA512}

    // Use deterministic builder!
    builder := obj.NewUpdatePlanBuilder(stage).
        WithTimestamp(params.Timestamp).
        WithVersionNum(obj.Inventory().Head.Num() + 1)

    plan, err := builder.Build(params.Message, &params.User)
    if err != nil {
        return nil, err
    }

    return plan.Activities(), nil
}

func ExecuteOCFLActivity(ctx context.Context, params ExecuteActivityParams) (ocfl.ActivityResult, error) {
    activity.RecordHeartbeat(ctx, "starting")

    objFS := os.DirFS(params.ObjectPath)
    contentSource := &ocfl.DirContentSource{FS: os.DirFS(params.ContentPath)}

    result, err := ocfl.ExecuteActivity(ctx, &params.Activity, objFS, params.ObjectPath, contentSource)

    if params.Activity.Type == ocfl.ActivityCopyContent {
        activity.RecordHeartbeat(ctx, result.BytesWritten)
    }

    return result, err
}
```

**Benefits:**
- ✅ **Automatic state persistence** - Temporal handles all state management
- ✅ **Configurable retries per activity** - Fine-grained retry policies
- ✅ **Distributed execution** - Activities run on any available worker
- ✅ **Progress queries** - Can query workflow state at any time
- ✅ **Deterministic replay** - Workflow can replay from any point
- ✅ **Heartbeats** - Long-running activities stay alive
- ✅ **Visibility** - Built-in UI shows progress, history, and errors
- ✅ **Version control** - Can safely deploy new workflow versions

### Starting the Workflow

```go
func startUpdateWorkflow(client client.Client, objectID, contentPath string) error {
    opts := client.StartWorkflowOptions{
        ID:        fmt.Sprintf("update-object-%s-%d", objectID, time.Now().Unix()),
        TaskQueue: "ocfl-updates",
    }

    params := UpdateParams{
        ObjectPath:  "/storage/objects/" + objectID,
        ContentPath: contentPath,
        Message:     "Updated via Temporal",
        User: ocfl.User{
            Name:    "Automated System",
            Address: "mailto:system@example.com",
        },
    }

    we, err := client.ExecuteWorkflow(context.Background(), opts, UpdateObjectWorkflow, params)
    if err != nil {
        return err
    }

    log.Printf("Started workflow: %s, Run ID: %s", we.GetID(), we.GetRunID())

    // Can wait for result or return immediately
    return we.Get(context.Background(), nil)
}
```

### Querying Workflow Progress

```go
// Query workflow status at any time
func checkUpdateProgress(client client.Client, workflowID string) error {
    desc, err := client.DescribeWorkflowExecution(context.Background(), workflowID, "")
    if err != nil {
        return err
    }

    fmt.Printf("Status: %s\n", desc.WorkflowExecutionInfo.Status)
    fmt.Printf("Start Time: %s\n", desc.WorkflowExecutionInfo.StartTime)

    // Get workflow history to see which activities completed
    iter := client.GetWorkflowHistory(context.Background(), workflowID, "", false, 0)
    for iter.HasNext() {
        event, err := iter.Next()
        if err != nil {
            return err
        }
        if event.EventType == enumspb.EVENT_TYPE_ACTIVITY_TASK_COMPLETED {
            fmt.Printf("✓ Activity completed: %s\n", event.GetActivityTaskCompletedEventAttributes().ActivityType.Name)
        }
    }

    return nil
}
```

---

## Durable Task Framework (Simpler Alternative)

### Orchestration Function

```go
// Lighter-weight alternative to Temporal
func UpdateObjectOrchestration(ctx *backend.OrchestrationContext) (any, error) {
    var params UpdateOrchestrationInput
    if err := ctx.GetInput(&params); err != nil {
        return nil, err
    }

    // Stage content
    var stageState ocfl.DigestMap
    err := ctx.CallActivity(BuildStageActivityName,
        backend.WithActivityInput(params.ContentPath)).Await(&stageState)
    if err != nil {
        return nil, err
    }

    // Create plan
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

    // Execute activities
    results := make([]ocfl.ActivityResult, 0, len(activities))
    for _, activity := range activities {
        var result ocfl.ActivityResult
        err = ctx.CallActivity(ExecuteActivityName,
            backend.WithActivityInput(ExecuteInput{
                ObjectPath:  params.ObjectPath,
                Activity:    activity,
                ContentPath: params.ContentPath,
            }),
        ).Await(&result)
        if err != nil {
            return nil, err
        }
        results = append(results, result)
    }

    return UpdateOrchestrationOutput{
        ActivitiesCompleted: len(activities),
        TotalBytesWritten:   sumBytes(results),
    }, nil
}
```

**Benefits:**
- ✅ **Embeddable** - No separate cluster to manage
- ✅ **Simpler ops** - Just a library in your process
- ✅ **Still durable** - Automatic state persistence
- ✅ **Replay-safe** - Deterministic execution
- ⚠️ Less feature-rich than Temporal (no advanced queries, signals, etc.)

### Starting Orchestration

```go
func runUpdate(ctx context.Context, backend backend.Backend, params UpdateOrchestrationInput) error {
    client := backend.GetClient()

    id, err := client.ScheduleNewOrchestration(ctx, "UpdateObject",
        api.WithInput(params),
        api.WithInstanceID(fmt.Sprintf("update-%s-%d", params.ObjectID, time.Now().Unix())))
    if err != nil {
        return err
    }

    log.Printf("Started orchestration: %s", id)

    // Wait for completion
    metadata, err := client.WaitForOrchestrationCompletion(ctx, id)
    if err != nil {
        return err
    }

    var output UpdateOrchestrationOutput
    if err := metadata.ReadOutput(&output); err != nil {
        return err
    }

    log.Printf("Update complete: %d activities, %d bytes written",
        output.ActivitiesCompleted, output.TotalBytesWritten)

    return nil
}
```

---

## Feature Comparison Matrix

| Feature | Current API | Temporal | Durable Task |
|---------|-------------|----------|--------------|
| **State Persistence** | Manual (MarshalBinary) | Automatic | Automatic |
| **Resumability** | Manual restore | Automatic | Automatic |
| **Retries** | Manual | Per-activity config | Built-in |
| **Distributed Execution** | ❌ | ✅ Workers across machines | ⚠️ Single process |
| **Progress Tracking** | Manual | Built-in queries | Basic |
| **Visibility UI** | ❌ | ✅ Temporal Web UI | Limited |
| **Deterministic Time** | ❌ Uses time.Now() | ✅ workflow.Now() | ✅ ctx.CurrentTimeUtc |
| **Heartbeats** | ❌ | ✅ Activity heartbeats | ⚠️ Basic |
| **Signals** | ❌ | ✅ External signals | ⚠️ Limited |
| **Versioning** | ❌ | ✅ Workflow versioning | ⚠️ Basic |
| **Operational Complexity** | Low | High (cluster required) | Low (embeddable) |
| **Learning Curve** | Simple | Moderate | Simple |
| **Step-level Control** | ❌ | ✅ | ✅ |
| **Concurrent Activities** | ⚠️ Internal goroutines | ✅ Explicit parallel | ✅ Explicit parallel |

---

## Use Case Recommendations

### Use Current API When:
- Simple, short-lived updates (< 1 minute)
- Single-machine deployment
- No need for progress tracking
- No distributed workers
- Minimal operational complexity preferred

### Use Temporal When:
- Long-running updates (hours/days)
- Large-scale distributed system
- Need advanced features (signals, queries, versioning)
- Team comfortable with operational complexity
- Need strong visibility and monitoring
- Multiple workers across machines

### Use Durable Task Framework When:
- Medium-duration updates (minutes to hours)
- Want durability without operational overhead
- Single-process deployment acceptable
- Need deterministic execution
- Simpler than Temporal but more than current API

---

## Migration Strategy

All three approaches can coexist:

```go
// Option 1: Simple update (current API)
if simpleUpdate {
    return obj.Update(ctx, stage, msg, user)
}

// Option 2: Temporal workflow
if useWorkflows {
    return startTemporalWorkflow(client, params)
}

// Option 3: Durable Task orchestration
if useDurableTask {
    return runDurableOrchestration(ctx, backend, params)
}
```

No breaking changes - new APIs are additive!
