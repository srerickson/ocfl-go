# Durable Task Framework Example

This example demonstrates using OCFL object updates with **Microsoft's Durable Task Framework**, a lightweight, embeddable orchestration engine for Go.

## What is Durable Task Framework?

The Durable Task Framework is:
- **Embeddable** - Runs in your process, no separate server cluster needed
- **Lightweight** - Simpler than Temporal, fewer moving parts
- **Durable** - Automatic state persistence and recovery
- **Replay-safe** - Deterministic execution with event sourcing

It's a great middle ground between manual state management and full workflow engines like Temporal.

## What This Example Shows

This example demonstrates:

1. **Orchestration-based updates** - Multi-step workflows with automatic state management
2. **Deterministic execution** - Uses `ctx.CurrentTimeUtc()` for replay safety
3. **Activity isolation** - Each step is a separate, retryable activity
4. **Automatic persistence** - State saved to SQLite database
5. **Built-in recovery** - Orchestrations resume after crashes
6. **Progress tracking** - Full visibility into execution state

## Architecture

### Orchestration Flow

```
UpdateObjectOrchestration (deterministic workflow logic)
    ↓
BuildStageActivity (compute content digests)
    ↓
CreatePlanActivity (create update plan with deterministic timestamp)
    ↓
For each activity in plan:
    ExecuteOCFLActivity (execute individual step)
    ↓
ApplyPlanActivity (finalize update)
```

### Key Components

**Orchestration** (`UpdateObjectOrchestration`):
- Deterministic workflow logic
- Cannot do I/O directly
- Uses framework's time (`ctx.CurrentTimeUtc()`)
- Persisted as event log

**Activities**:
- `BuildStageActivity` - Compute file digests
- `CreatePlanActivity` - Build update plan
- `ExecuteOCFLActivity` - Execute individual update step
- `ApplyPlanActivity` - Finalize the update

**Backend**:
- SQLite for state persistence
- Stores orchestration history
- Enables crash recovery

## Running the Example

### Prerequisites

```bash
go get github.com/microsoft/durabletask-go@latest
```

### Run

```bash
cd examples/durable-task
go run main.go
```

### Expected Output

```
🚀 OCFL Update with Durable Task Framework
=============================================

📊 Initializing Durable Task Framework...
   ✅ Workers started

🎯 Starting orchestration: update-test:durable-task-example-1234567890

⏳ Waiting for orchestration to complete...
   (The framework ensures durability across failures)

[INFO] Starting update for object: test:durable-task-example
[INFO] Staged 4 files
[INFO] Plan has 10 activities
[INFO] [1/10] Completed: write 0=ocfl_object_1.1 (17 bytes)
[INFO] [2/10] Completed: version directory v1 (0 bytes)
[INFO] [3/10] Completed: copy v1/content/data/file1.txt (19 bytes)
[INFO] [4/10] Completed: copy v1/content/data/file2.txt (19 bytes)
[INFO] [5/10] Completed: copy v1/content/metadata.json (57 bytes)
[INFO] [6/10] Completed: copy v1/content/readme.txt (50 bytes)
[INFO] [7/10] Completed: write v1/inventory.json (1531 bytes)
[INFO] [8/10] Completed: write v1/inventory.json.sha512 (128 bytes)
[INFO] [9/10] Completed: write inventory.json (1531 bytes)
[INFO] [10/10] Completed: write inventory.json.sha512 (128 bytes)

✅ Orchestration completed successfully!

   Object ID: test:durable-task-example
   Version: v1
   Activities: 10
   Total bytes: 3480

💡 Try running again - the framework will detect it's already done!
   Or delete orchestration.db to start fresh
```

## Testing Durability

### Test 1: Crash Recovery

The Durable Task Framework automatically handles crashes:

1. **Start the update**:
   ```bash
   go run main.go
   ```

2. **Kill the process** (Ctrl+C) while it's running

3. **Restart**:
   ```bash
   go run main.go
   ```

The orchestration will **automatically resume** from where it left off! The framework replays the orchestration from the event log stored in SQLite.

### Test 2: Idempotency

Run the example multiple times:

```bash
go run main.go  # First run
go run main.go  # Second run - detects no changes needed
```

Activities are idempotent, so repeated execution is safe.

## Code Walkthrough

### Deterministic Orchestration

```go
func UpdateObjectOrchestration(ctx *task.OrchestrationContext) (any, error) {
    // ✅ Deterministic: uses framework's time, not time.Now()
    timestamp := ctx.CurrentTimeUtc()

    // ✅ Deterministic: all state changes via activities
    var stageState ocfl.DigestMap
    ctx.CallActivity(BuildStageActivityName, input).Await(&stageState)

    // ✅ Framework persists state after each activity
    for _, activity := range activities {
        ctx.CallActivity(ExecuteActivityName, activity).Await(&result)
    }
}
```

### Activity Pattern

```go
func ExecuteOCFLActivityFunc(ctx context.Context, input ExecuteActivityInput) (ocfl.ActivityResult, error) {
    // Activities can:
    // - Do I/O (read files, write files)
    // - Call external services
    // - Have side effects

    // The framework:
    // - Retries on failure
    // - Records completion
    // - Enables replay

    return ocfl.ExecuteActivity(ctx, input.Activity, fsys, ".", src)
}
```

## Comparison with Other Approaches

| Feature | Simple Example | Durable Task | Temporal |
|---------|---------------|--------------|----------|
| **Setup** | ⭐ None | ⭐⭐ SQLite | ⭐⭐⭐ Server cluster |
| **Complexity** | ⭐ Low | ⭐⭐ Medium | ⭐⭐⭐ High |
| **Durability** | Manual (JSON) | Automatic (SQLite) | Automatic (DB) |
| **Distribution** | Single process | Single process | Multi-worker |
| **Crash Recovery** | Manual resume | Automatic | Automatic |
| **Operational Cost** | None | Low | High |
| **Best For** | Learning | Production (small) | Enterprise |

## Key Advantages

### vs. Simple Example

- ✅ **Automatic state management** - No manual JSON files
- ✅ **Built-in recovery** - Crashes don't lose progress
- ✅ **Event sourcing** - Full execution history
- ✅ **Better observability** - Query orchestration state

### vs. Temporal

- ✅ **Simpler setup** - No server cluster to manage
- ✅ **Lower complexity** - Fewer concepts to learn
- ✅ **Embeddable** - Just a Go library
- ✅ **Good enough** - For many production use cases

## Production Considerations

### Scaling

For production use:

```go
// Use a proper database instead of SQLite
import "github.com/microsoft/durabletask-go/backend/postgres"

be := postgres.NewPostgresBackend(postgresOptions, logger)
```

### Monitoring

Add observability:

```go
func ExecuteOCFLActivityFunc(ctx context.Context, input ExecuteActivityInput) (ocfl.ActivityResult, error) {
    // Add metrics
    startTime := time.Now()
    defer func() {
        metrics.RecordDuration("ocfl_activity", time.Since(startTime))
    }()

    // Add tracing
    span := trace.StartSpan(ctx, input.Activity.Name)
    defer span.End()

    result, err := ocfl.ExecuteActivity(ctx, input.Activity, fsys, ".", src)

    // Record result
    if err != nil {
        metrics.RecordError("ocfl_activity", input.Activity.Type.String())
    } else {
        metrics.RecordBytes("ocfl_bytes_written", result.BytesWritten)
    }

    return result, err
}
```

### Error Handling

Configure retry policies:

```go
r := task.NewTaskRegistry()
r.AddOrchestratorN("UpdateObject", UpdateObjectOrchestration)

// Add retry policy for activities
r.AddActivityN(ExecuteActivityName, ExecuteOCFLActivityFunc,
    task.WithActivityRetryPolicy(&task.RetryPolicy{
        MaxAttempts:        5,
        InitialInterval:    time.Second,
        BackoffCoefficient: 2.0,
        MaxInterval:        time.Minute,
    }))
```

### Distributed Workers

For horizontal scaling:

```go
// Run multiple worker processes
// Each connects to the same backend database
orchestrator := backend.NewOrchestrationWorker(be, r)
taskWorker := backend.NewActivityTaskWorker(be, r)

// Framework coordinates work across workers
orchestrator.Start(ctx)
taskWorker.Start(ctx)
```

## Troubleshooting

### "Orchestration not found"

The orchestration was deleted or never created. Check:
- Orchestration ID is correct
- Backend database is accessible
- No errors during `ScheduleNewOrchestration`

### "Activity failed after retries"

An activity failed multiple times. Check:
- Activity implementation for bugs
- File permissions
- Disk space
- Network connectivity (if using remote storage)

### "Orchestration stuck"

The orchestration isn't making progress. Check:
- Workers are running (`orchestrator.Start()` and `taskWorker.Start()`)
- Backend database is accessible
- No deadlocks in activity code

## Cleanup

To clean up after running:

```bash
rm -rf example-storage example-content orchestration.db orchestration.db-shm orchestration.db-wal
```

## Next Steps

- Read the [Durable Task Framework documentation](https://github.com/microsoft/durabletask-go)
- See `examples/durable-simple/` for a framework-free approach
- See `docs/durable-execution-design.md` for architecture details
- Explore advanced features like signals, sub-orchestrations, and timers

## Learn More

- [Durable Task Framework on GitHub](https://github.com/microsoft/durabletask-go)
- [Durable Task Concepts](https://microsoft.github.io/durabletask-go/)
- [OCFL Specification](https://ocfl.io/)
