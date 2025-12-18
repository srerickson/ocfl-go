# Simple Durable Execution Example

This example demonstrates how to use OCFL's durable execution APIs to create resilient, resumable object updates without requiring a full workflow orchestration framework.

## What This Example Shows

1. **Deterministic Plan Creation** - Plans are created with explicit timestamps for reproducibility
2. **Activity-Based Execution** - Updates broken into individual, executable activities
3. **Idempotency** - Activities can be safely retried without side effects
4. **Progress Tracking** - Execution state is persisted between runs
5. **Resumability** - Interrupted updates can resume from where they left off

## Key Concepts

### Durable Execution Pattern

Unlike the traditional `Update()` API which executes all steps in one go, the durable execution pattern:

```go
// Traditional approach (all-or-nothing)
err := obj.Update(ctx, stage, msg, user)

// Durable approach (step-by-step with tracking)
builder := obj.NewUpdatePlanBuilder(stage).
    WithTimestamp(deterministicTime)
plan, _ := builder.Build(msg, user)

activities, _ := plan.Activities()
for _, activity := range activities {
    result, err := ocfl.ExecuteActivity(ctx, activity, fsys, ".", src)
    // Track progress, handle errors, save state
}
```

### Benefits

- **Resilient**: Failures don't require starting over
- **Observable**: Track progress of long-running updates
- **Testable**: Individual activities can be tested in isolation
- **Flexible**: Custom retry logic, rate limiting, etc.

## Running the Example

```bash
cd examples/durable-simple
go run main.go
```

### Expected Output

```
🚀 Starting Durable OCFL Object Update Example
================================================

✨ Starting new execution

📦 Staging content...
   Staged 4 files

📋 Creating update plan...
   Plan has 10 activities

⚡ Executing activities...
   (Activities are idempotent - safe to retry)

   [1/10] ✅ write 0=ocfl_object_1.1
   [2/10] ✅ version directory v1
   [3/10] ✅ copy v1/content/readme.txt
         📊 61 bytes written
   [4/10] ✅ copy v1/content/data/file1.txt
         📊 19 bytes written
   ...
   [10/10] ✅ write inventory.json.sha512

🎯 Finalizing update...

✅ Update completed successfully!
   Object ID: test:durable-example
   New version: v1
   Total activities: 10
   Bytes written: 234

💡 Try running again to see idempotency!
   Or interrupt execution (Ctrl+C) and restart to see resume functionality
```

## Testing Resumability

To see the resume functionality in action:

1. **Start the update:**
   ```bash
   go run main.go
   ```

2. **Interrupt it** (press Ctrl+C) while it's running

3. **Restart it:**
   ```bash
   go run main.go
   ```

You'll see output like:
```
📂 Resuming execution (completed 5/10 activities)
...
   [1/10] ⏭️  Skipping (already done): write 0=ocfl_object_1.1
   [2/10] ⏭️  Skipping (already done): version directory v1
   ...
   [6/10] ✅ copy v1/content/metadata.json
```

## Testing Idempotency

Run the example multiple times:

```bash
go run main.go  # First run - creates everything
go run main.go  # Second run - all activities skipped
```

On subsequent runs, you'll see all activities are skipped because they detect the files already exist with correct content:

```
   [1/10] ⏭️  Skipping (already done): write 0=ocfl_object_1.1
         💡 declaration file already exists
```

## Architecture

### State Management

The example persists execution state to `execution-state.json`:

```json
{
  "object_id": "test:durable-example",
  "plan_created_at": "2025-01-15T10:30:00Z",
  "completed_steps": {
    "write 0=ocfl_object_1.1": true,
    "copy v1/content/readme.txt": true
  },
  "total_activities": 10,
  "completed_count": 2
}
```

This state file enables:
- **Resume** after interruption
- **Progress tracking** for monitoring
- **Auditing** of what was completed when

### Error Handling

The example includes retry logic:

```go
for attempt := 1; attempt <= 3; attempt++ {
    result, err = ocfl.ExecuteActivity(ctx, activity, ...)
    if err == nil {
        break
    }
    time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
}
```

This demonstrates how you could integrate with:
- Exponential backoff
- Circuit breakers
- Rate limiters
- Metrics collection

## Adapting for Production

To use this pattern in production, you might:

1. **Use a database** instead of JSON file for state:
   ```go
   // Store in PostgreSQL, MongoDB, etc.
   db.SaveActivityResult(objectID, activity.Name, result)
   ```

2. **Add distributed locking**:
   ```go
   lock := redisLock.Acquire(ctx, objectID)
   defer lock.Release()
   ```

3. **Implement distributed coordination**:
   - Use message queues (RabbitMQ, Kafka)
   - Use distributed tasks (Celery, Machinery)
   - Use workflow engines (Temporal, Cadence)

4. **Add observability**:
   ```go
   metrics.RecordActivity(activity.Type, result.BytesWritten)
   tracing.RecordSpan(ctx, activity.Name)
   ```

## Comparison with Frameworks

| Feature | This Example | Temporal | Durable Task |
|---------|--------------|----------|--------------|
| **Complexity** | Low | High | Medium |
| **Dependencies** | None | Server cluster | Library only |
| **Durability** | Manual (JSON) | Automatic | Automatic |
| **Distribution** | Single process | Multi-worker | Multi-worker |
| **Replay** | Manual | Automatic | Automatic |
| **Best For** | Learning, simple cases | Complex workflows | Embedded workflows |

## Next Steps

- See `examples/temporal/` for a full Temporal integration example
- See `examples/durable-task/` for Microsoft Durable Task Framework example
- Read `docs/durable-execution-design.md` for architecture details

## Cleanup

To clean up after running the example:

```bash
rm -rf example-storage example-content execution-state.json
```
