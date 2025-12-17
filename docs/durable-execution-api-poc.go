package ocfl

// This is a proof-of-concept showing proposed API additions for durable execution frameworks.
// This file is not meant to compile as-is, but to illustrate the design.

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "io/fs"
    "time"

    "github.com/srerickson/ocfl-go/digest"
)

// ========================================
// New Types for Durable Execution
// ========================================

// ActivityType identifies the type of update activity
type ActivityType int

const (
    // ActivityDeclareObject creates the NAMASTE declaration file
    ActivityDeclareObject ActivityType = iota
    // ActivityCreateVersionDir creates the version directory
    ActivityCreateVersionDir
    // ActivityCopyContent copies a content file to the object
    ActivityCopyContent
    // ActivityWriteVersionInventory writes the version inventory.json
    ActivityWriteVersionInventory
    // ActivityWriteVersionSidecar writes the version inventory sidecar
    ActivityWriteVersionSidecar
    // ActivityWriteRootInventory writes the root inventory.json
    ActivityWriteRootInventory
    // ActivityWriteRootSidecar writes the root inventory sidecar
    ActivityWriteRootSidecar
)

func (at ActivityType) String() string {
    switch at {
    case ActivityDeclareObject:
        return "DeclareObject"
    case ActivityCreateVersionDir:
        return "CreateVersionDir"
    case ActivityCopyContent:
        return "CopyContent"
    case ActivityWriteVersionInventory:
        return "WriteVersionInventory"
    case ActivityWriteVersionSidecar:
        return "WriteVersionSidecar"
    case ActivityWriteRootInventory:
        return "WriteRootInventory"
    case ActivityWriteRootSidecar:
        return "WriteRootSidecar"
    default:
        return fmt.Sprintf("Unknown(%d)", at)
    }
}

// UpdatePlanActivity represents a single executable step in an update plan.
// Unlike internal PlanSteps, these are serializable and framework-agnostic.
type UpdatePlanActivity struct {
    // Name is a unique identifier for this activity (e.g., "copy-content-sha512-abc123")
    Name string `json:"name"`

    // Type identifies what kind of activity this is
    Type ActivityType `json:"type"`

    // ContentDigest is the digest of content being copied (for ActivityCopyContent)
    ContentDigest string `json:"content_digest,omitempty"`

    // Size is the expected size in bytes (for ActivityCopyContent)
    Size int64 `json:"size,omitempty"`

    // Params contains type-specific parameters
    Params ActivityParams `json:"params"`
}

// ActivityParams holds parameters for different activity types
type ActivityParams struct {
    // For ActivityDeclareObject
    Spec Spec `json:"spec,omitempty"`

    // For ActivityCreateVersionDir
    VersionPath string `json:"version_path,omitempty"`

    // For ActivityCopyContent
    SourceDigest string `json:"source_digest,omitempty"`
    DestPath     string `json:"dest_path,omitempty"`

    // For ActivityWriteVersionInventory, ActivityWriteRootInventory
    InventoryJSON []byte `json:"inventory_json,omitempty"`

    // For ActivityWriteVersionSidecar, ActivityWriteRootSidecar
    SidecarPath    string `json:"sidecar_path,omitempty"`
    SidecarContent []byte `json:"sidecar_content,omitempty"`
}

// ActivityResult contains the result of executing an activity
type ActivityResult struct {
    // BytesWritten is the number of bytes written (for copy operations)
    BytesWritten int64 `json:"bytes_written"`

    // DigestComputed is the digest of content written (for verification)
    DigestComputed string `json:"digest_computed,omitempty"`

    // Error contains any error that occurred
    Error error `json:"error,omitempty"`

    // Skipped indicates the activity was skipped (e.g., file already exists)
    Skipped bool `json:"skipped,omitempty"`

    // SkipReason explains why the activity was skipped
    SkipReason string `json:"skip_reason,omitempty"`
}

// ========================================
// UpdatePlanBuilder - Deterministic Plan Creation
// ========================================

// UpdatePlanBuilder creates update plans using externally-provided values.
// This enables deterministic plan creation required by durable execution frameworks.
type UpdatePlanBuilder struct {
    obj       *Object
    stage     *Stage
    timestamp time.Time
    versionNum int
    contentPathFunc ContentPathFunc
    fixitySource FixitySource
    spec Spec
}

// NewUpdatePlanBuilder creates a builder for deterministic plan creation.
// Unlike NewUpdatePlan, this doesn't automatically use time.Now() or generate
// random values, making it suitable for replay-safe workflows.
func (obj *Object) NewUpdatePlanBuilder(stage *Stage) *UpdatePlanBuilder {
    return &UpdatePlanBuilder{
        obj:   obj,
        stage: stage,
        versionNum: obj.Inventory().Head.Num() + 1, // default next version
        spec:  obj.Inventory().Spec, // inherit spec
    }
}

// WithTimestamp sets the creation timestamp for the new version.
// In durable execution frameworks, this should come from workflow.Now() or
// ctx.CurrentTimeUtc to ensure determinism during replay.
func (b *UpdatePlanBuilder) WithTimestamp(t time.Time) *UpdatePlanBuilder {
    b.timestamp = t
    return b
}

// WithVersionNum explicitly sets the version number.
// Useful for distributed systems where version numbers need coordination.
func (b *UpdatePlanBuilder) WithVersionNum(vnum int) *UpdatePlanBuilder {
    b.versionNum = vnum
    return b
}

// WithContentPathFunc sets a custom function for generating content paths
func (b *UpdatePlanBuilder) WithContentPathFunc(fn ContentPathFunc) *UpdatePlanBuilder {
    b.contentPathFunc = fn
    return b
}

// WithFixitySource sets the fixity source for alternate digests
func (b *UpdatePlanBuilder) WithFixitySource(src FixitySource) *UpdatePlanBuilder {
    b.fixitySource = src
    return b
}

// WithSpec sets the OCFL spec version for the update
func (b *UpdatePlanBuilder) WithSpec(spec Spec) *UpdatePlanBuilder {
    b.spec = spec
    return b
}

// Build creates the update plan using the configured parameters.
// This method must be deterministic - it should not read the clock,
// generate random values, or perform I/O.
func (b *UpdatePlanBuilder) Build(msg string, user *User, opts ...UpdateOption) (*UpdatePlan, error) {
    // Validate timestamp was set
    if b.timestamp.IsZero() {
        return nil, fmt.Errorf("timestamp must be set via WithTimestamp()")
    }

    // Create new inventory using builder pattern
    invBuilder := b.obj.InventoryBuilder()

    if b.contentPathFunc != nil {
        invBuilder = invBuilder.ContentPathFunc(b.contentPathFunc)
    }
    if b.fixitySource != nil {
        invBuilder = invBuilder.FixitySource(b.fixitySource)
    }

    invBuilder = invBuilder.Spec(b.spec).
        AddVersion(b.stage.State, b.stage.DigestAlgorithm, b.timestamp, msg, user)

    newInv, err := invBuilder.Finalize()
    if err != nil {
        return nil, fmt.Errorf("building inventory: %w", err)
    }

    // Create plan from inventories
    plan := &UpdatePlan{
        baseInventory: b.obj.Inventory(),
        newInventory:  newInv,
        activities:    make([]*UpdatePlanActivity, 0),
    }

    // Generate activities from inventory diff
    if err := plan.generateActivities(b.obj, b.spec); err != nil {
        return nil, fmt.Errorf("generating activities: %w", err)
    }

    return plan, nil
}

// ========================================
// UpdatePlan Extensions
// ========================================

// Activities returns all activities in execution order.
// This is the primary interface for durable execution frameworks.
func (plan *UpdatePlan) Activities() []*UpdatePlanActivity {
    return plan.activities
}

// ActivityByName retrieves a specific activity by its unique name
func (plan *UpdatePlan) ActivityByName(name string) (*UpdatePlanActivity, error) {
    for _, activity := range plan.activities {
        if activity.Name == name {
            return activity, nil
        }
    }
    return nil, fmt.Errorf("activity %q not found", name)
}

// MarkActivityComplete records that an activity has completed successfully.
// This is called by framework activities after successful execution.
func (plan *UpdatePlan) MarkActivityComplete(activityName string, result ActivityResult) error {
    activity, err := plan.ActivityByName(activityName)
    if err != nil {
        return err
    }

    if result.Error != nil {
        return fmt.Errorf("activity %q failed: %w", activityName, result.Error)
    }

    // Mark as completed in internal state
    // (implementation would update internal tracking)
    _ = activity // use activity

    return nil
}

// generateActivities creates the list of activities from inventory changes
func (plan *UpdatePlan) generateActivities(obj *Object, spec Spec) error {
    newVersion := plan.newInventory.Head
    versionPath := newVersion.String()

    // Activity 1: Declare object (NAMASTE file)
    if obj.Inventory().Head.Num() == 0 {
        plan.activities = append(plan.activities, &UpdatePlanActivity{
            Name: "declare-object",
            Type: ActivityDeclareObject,
            Params: ActivityParams{
                Spec: spec,
            },
        })
    }

    // Activity 2: Create version directory
    plan.activities = append(plan.activities, &UpdatePlanActivity{
        Name: fmt.Sprintf("create-version-%s", versionPath),
        Type: ActivityCreateVersionDir,
        Params: ActivityParams{
            VersionPath: versionPath,
        },
    })

    // Activity 3+: Copy content files
    // Find new content (digests in new manifest but not in base manifest)
    for contentDigest, paths := range plan.newInventory.Manifest {
        // Check if this digest exists in base
        if _, exists := plan.baseInventory.Manifest[contentDigest]; exists {
            continue // already have this content
        }

        // Need to copy this content
        for _, destPath := range paths {
            plan.activities = append(plan.activities, &UpdatePlanActivity{
                Name:          fmt.Sprintf("copy-content-%s", contentDigest[:16]),
                Type:          ActivityCopyContent,
                ContentDigest: contentDigest,
                Params: ActivityParams{
                    SourceDigest: contentDigest,
                    DestPath:     destPath,
                },
            })
            break // only need to copy once
        }
    }

    // Activity N-3: Write version inventory
    invJSON, err := json.Marshal(plan.newInventory)
    if err != nil {
        return fmt.Errorf("marshaling inventory: %w", err)
    }

    plan.activities = append(plan.activities, &UpdatePlanActivity{
        Name: fmt.Sprintf("write-version-inventory-%s", versionPath),
        Type: ActivityWriteVersionInventory,
        Params: ActivityParams{
            VersionPath:   versionPath,
            InventoryJSON: invJSON,
        },
    })

    // Activity N-2: Write version sidecar
    sidecarContent := []byte(plan.newInventory.DigestAlgorithm.Sum(invJSON))
    plan.activities = append(plan.activities, &UpdatePlanActivity{
        Name: fmt.Sprintf("write-version-sidecar-%s", versionPath),
        Type: ActivityWriteVersionSidecar,
        Params: ActivityParams{
            SidecarPath:    fmt.Sprintf("%s/inventory.json.%s", versionPath, plan.newInventory.DigestAlgorithm),
            SidecarContent: sidecarContent,
        },
    })

    // Activity N-1: Write root inventory
    plan.activities = append(plan.activities, &UpdatePlanActivity{
        Name: "write-root-inventory",
        Type: ActivityWriteRootInventory,
        Params: ActivityParams{
            InventoryJSON: invJSON,
        },
    })

    // Activity N: Write root sidecar
    plan.activities = append(plan.activities, &UpdatePlanActivity{
        Name: "write-root-sidecar",
        Type: ActivityWriteRootSidecar,
        Params: ActivityParams{
            SidecarPath:    fmt.Sprintf("inventory.json.%s", plan.newInventory.DigestAlgorithm),
            SidecarContent: sidecarContent,
        },
    })

    return nil
}

// ========================================
// Activity Executor
// ========================================

// ExecuteActivity executes a single UpdatePlanActivity.
// This is called by durable execution framework activities.
func ExecuteActivity(
    ctx context.Context,
    activity *UpdatePlanActivity,
    objFS fs.FS,
    objPath string,
    src ContentSource,
) (ActivityResult, error) {
    switch activity.Type {
    case ActivityDeclareObject:
        return executeDeclareObject(ctx, objFS, objPath, activity)

    case ActivityCreateVersionDir:
        return executeCreateVersionDir(ctx, objFS, objPath, activity)

    case ActivityCopyContent:
        return executeCopyContent(ctx, objFS, objPath, src, activity)

    case ActivityWriteVersionInventory:
        return executeWriteInventory(ctx, objFS, objPath, activity, true)

    case ActivityWriteVersionSidecar:
        return executeWriteSidecar(ctx, objFS, objPath, activity, true)

    case ActivityWriteRootInventory:
        return executeWriteInventory(ctx, objFS, objPath, activity, false)

    case ActivityWriteRootSidecar:
        return executeWriteSidecar(ctx, objFS, objPath, activity, false)

    default:
        return ActivityResult{}, fmt.Errorf("unknown activity type: %v", activity.Type)
    }
}

// Individual activity executors (simplified implementations)

func executeDeclareObject(ctx context.Context, objFS fs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
    // Write NAMASTE declaration file
    // Implementation would create the 0=ocfl_object_1.x file
    return ActivityResult{BytesWritten: 0}, nil
}

func executeCreateVersionDir(ctx context.Context, objFS fs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
    // Create version directory
    // Implementation would use os.MkdirAll or similar
    return ActivityResult{BytesWritten: 0}, nil
}

func executeCopyContent(ctx context.Context, objFS fs.FS, objPath string, src ContentSource, activity *UpdatePlanActivity) (ActivityResult, error) {
    // Get content from source
    contentFS, path := src.GetContent(activity.ContentDigest)
    if contentFS == nil {
        return ActivityResult{}, fmt.Errorf("content not found: %s", activity.ContentDigest)
    }

    // Open source file
    srcFile, err := contentFS.Open(path)
    if err != nil {
        return ActivityResult{}, fmt.Errorf("opening source: %w", err)
    }
    defer srcFile.Close()

    // Create destination file
    // (Implementation would handle proper path construction and fs.FS compatibility)

    // Copy with digest verification
    digester := digest.Sum(activity.Params.SourceDigest) // get algorithm from digest
    written, err := io.Copy(io.MultiWriter(io.Discard /* destFile */, digester), srcFile)
    if err != nil {
        return ActivityResult{}, fmt.Errorf("copying content: %w", err)
    }

    computed := digester.String()
    if computed != activity.ContentDigest {
        return ActivityResult{}, fmt.Errorf("digest mismatch: expected %s, got %s",
            activity.ContentDigest, computed)
    }

    return ActivityResult{
        BytesWritten:   written,
        DigestComputed: computed,
    }, nil
}

func executeWriteInventory(ctx context.Context, objFS fs.FS, objPath string, activity *UpdatePlanActivity, isVersion bool) (ActivityResult, error) {
    // Write inventory.json file
    // Implementation would handle path construction and writing
    bytesWritten := int64(len(activity.Params.InventoryJSON))
    return ActivityResult{BytesWritten: bytesWritten}, nil
}

func executeWriteSidecar(ctx context.Context, objFS fs.FS, objPath string, activity *UpdatePlanActivity, isVersion bool) (ActivityResult, error) {
    // Write sidecar file
    bytesWritten := int64(len(activity.Params.SidecarContent))
    return ActivityResult{BytesWritten: bytesWritten}, nil
}

// ========================================
// Helper Types
// ========================================

// UpdatePlan extensions (add to existing type)
type UpdatePlan struct {
    baseInventory *Inventory
    newInventory  *Inventory
    activities    []*UpdatePlanActivity
    // ... existing fields ...
}
