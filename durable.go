package ocfl

import (
	"context"
	"fmt"
	"time"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// ActivityType identifies the type of update activity.
// Activities represent individual steps in an OCFL object update that can be
// executed independently by durable execution frameworks.
type ActivityType int

const (
	// ActivityDeclareObject creates the NAMASTE declaration file (0=ocfl_object_1.x)
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
	// ActivityRemoveOldDeclaration removes an old OCFL spec declaration file
	ActivityRemoveOldDeclaration
)

// String returns a human-readable name for the activity type
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
	case ActivityRemoveOldDeclaration:
		return "RemoveOldDeclaration"
	default:
		return fmt.Sprintf("Unknown(%d)", at)
	}
}

// UpdatePlanActivity represents a single executable step in an update plan.
// Unlike internal PlanSteps, UpdatePlanActivities are designed to be:
// - Serializable to JSON
// - Framework-agnostic
// - Executable in distributed environments
// - Idempotent when possible
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

// ActivityParams holds parameters for different activity types.
// Fields are used based on the ActivityType.
type ActivityParams struct {
	// For ActivityDeclareObject, ActivityRemoveOldDeclaration
	Spec         Spec   `json:"spec,omitempty"`
	SpecVersion  string `json:"spec_version,omitempty"` // e.g., "1.1"
	DeclarationFile string `json:"declaration_file,omitempty"` // e.g., "0=ocfl_object_1.1"

	// For ActivityCreateVersionDir
	VersionPath string `json:"version_path,omitempty"` // e.g., "v1", "v2"

	// For ActivityCopyContent
	SourceDigest string `json:"source_digest,omitempty"`
	DestPath     string `json:"dest_path,omitempty"`

	// For inventory write activities
	InventoryJSON   []byte `json:"inventory_json,omitempty"`
	InventoryDigest string `json:"inventory_digest,omitempty"`

	// For sidecar write activities
	SidecarPath      string `json:"sidecar_path,omitempty"`
	SidecarContent   []byte `json:"sidecar_content,omitempty"`
	DigestAlgorithm  string `json:"digest_algorithm,omitempty"`
}

// ActivityResult contains the result of executing an activity
type ActivityResult struct {
	// BytesWritten is the number of bytes written (for copy/write operations)
	BytesWritten int64 `json:"bytes_written"`

	// DigestComputed is the digest of content written (for verification)
	DigestComputed string `json:"digest_computed,omitempty"`

	// Error contains any error that occurred (nil if successful)
	Error error `json:"error,omitempty"`

	// Skipped indicates the activity was skipped (e.g., file already exists with correct content)
	Skipped bool `json:"skipped,omitempty"`

	// SkipReason explains why the activity was skipped
	SkipReason string `json:"skip_reason,omitempty"`
}

// UpdatePlanBuilder creates update plans using externally-provided values.
// This enables deterministic plan creation required by durable execution frameworks.
//
// Unlike NewUpdatePlan, UpdatePlanBuilder doesn't automatically use time.Now() or
// generate values that would be non-deterministic during workflow replay.
//
// Example usage with Temporal:
//
//	builder := obj.NewUpdatePlanBuilder(stage).
//	    WithTimestamp(workflow.Now(ctx)).  // Deterministic time from workflow
//	    WithVersionNum(obj.Inventory().Head.Num() + 1)
//	plan, err := builder.Build(msg, &user)
type UpdatePlanBuilder struct {
	obj             *Object
	stage           *Stage
	timestamp       time.Time
	versionNum      int
	contentPathFunc PathMutation
	fixitySource    FixitySource
	spec            Spec
	timestampSet    bool
}

// NewUpdatePlanBuilder creates a builder for deterministic plan creation.
// The builder requires an explicit timestamp to be set via WithTimestamp() before
// calling Build().
func (obj *Object) NewUpdatePlanBuilder(stage *Stage) *UpdatePlanBuilder {
	vnum := 1
	spec := Spec1_1 // default spec for new objects
	if obj.inventory != nil {
		vnum = obj.inventory.Head.Num() + 1
		spec = obj.inventory.Type.Spec
	}

	return &UpdatePlanBuilder{
		obj:        obj,
		stage:      stage,
		versionNum: vnum,
		spec:       spec,
	}
}

// WithTimestamp sets the creation timestamp for the new version.
// This is REQUIRED before calling Build().
//
// In durable execution frameworks, this should come from the framework's
// deterministic time source:
//   - Temporal: workflow.Now(ctx)
//   - Durable Task Framework: ctx.CurrentTimeUtc
func (b *UpdatePlanBuilder) WithTimestamp(t time.Time) *UpdatePlanBuilder {
	b.timestamp = t
	b.timestampSet = true
	return b
}

// WithVersionNum explicitly sets the version number for the new version.
// This is optional - if not set, it defaults to the next sequential version.
func (b *UpdatePlanBuilder) WithVersionNum(vnum int) *UpdatePlanBuilder {
	b.versionNum = vnum
	return b
}

// WithContentPathFunc sets a custom function for generating content paths
// in the version directory.
func (b *UpdatePlanBuilder) WithContentPathFunc(fn PathMutation) *UpdatePlanBuilder {
	b.contentPathFunc = fn
	return b
}

// WithFixitySource sets the fixity source for alternate digest checksums
func (b *UpdatePlanBuilder) WithFixitySource(src FixitySource) *UpdatePlanBuilder {
	b.fixitySource = src
	return b
}

// WithSpec sets the OCFL spec version for the update.
// If not set, inherits the spec from the existing object or defaults to 1.1.
func (b *UpdatePlanBuilder) WithSpec(spec Spec) *UpdatePlanBuilder {
	b.spec = spec
	return b
}

// Build creates the update plan using the configured parameters.
// This method must be deterministic - it should not read the clock,
// generate random values, or perform I/O.
//
// Returns an error if:
//   - Timestamp was not set via WithTimestamp()
//   - The stage state is unchanged from the current version (unless allowUnchanged option is used)
//   - Inventory construction fails
func (b *UpdatePlanBuilder) Build(msg string, user *User, opts ...ObjectUpdateOption) (*UpdatePlan, error) {
	// Validate timestamp was set
	if !b.timestampSet {
		return nil, fmt.Errorf("timestamp must be set via WithTimestamp() for deterministic plan creation")
	}

	// Build the new inventory
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

	// Check for unchanged state (unless explicitly allowed)
	updateOpts := newObjectUpdateOptions(opts...)
	currentInv := b.obj.inventory
	if !updateOpts.allowUnchanged && currentInv != nil {
		lastV := currentInv.Versions[currentInv.Head]
		if lastV != nil && lastV.State.Eq(b.stage.State) {
			return nil, fmt.Errorf("update has unchanged version state")
		}
	}

	// Create the plan
	plan, err := newUpdatePlan(newInv, currentInv)
	if err != nil {
		return nil, fmt.Errorf("creating update plan: %w", err)
	}

	plan.setGoLimit(updateOpts.goLimit)
	plan.setLogger(updateOpts.logger)

	return plan, nil
}

// Activities returns all activities in the update plan in execution order.
// This is the primary interface for durable execution frameworks to obtain
// the list of steps to execute.
//
// Each activity can be executed independently via ExecuteActivity().
func (plan *UpdatePlan) Activities() ([]*UpdatePlanActivity, error) {
	activities := make([]*UpdatePlanActivity, 0, len(plan.steps))

	for i := range plan.steps {
		step := &plan.steps[i]
		activity, err := planStepToActivity(step, plan.newInv, plan.oldInv)
		if err != nil {
			return nil, fmt.Errorf("converting step %q to activity: %w", step.Name(), err)
		}
		if activity != nil {
			activities = append(activities, activity)
		}
	}

	return activities, nil
}

// planStepToActivity converts a PlanStep to an UpdatePlanActivity.
// Returns nil if the step is not a content operation (e.g., noop steps).
func planStepToActivity(step *PlanStep, newInv, oldInv *StoredInventory) (*UpdatePlanActivity, error) {
	name := step.Name()

	// Determine activity type from step name patterns
	switch {
	case step.ContentDigest() != "":
		// This is a content copy step
		return &UpdatePlanActivity{
			Name:          name,
			Type:          ActivityCopyContent,
			ContentDigest: step.ContentDigest(),
			Size:          step.Size(),
			Params: ActivityParams{
				SourceDigest: step.ContentDigest(),
				DestPath:     extractDestPath(name),
			},
		}, nil

	case contains(name, "write 0=ocfl_object"):
		// Object declaration step
		spec := newInv.Type.Spec
		return &UpdatePlanActivity{
			Name: name,
			Type: ActivityDeclareObject,
			Params: ActivityParams{
				Spec:            spec,
				SpecVersion:     string(spec),
				DeclarationFile: extractFilename(name),
			},
		}, nil

	case contains(name, "remove 0=ocfl_object"):
		// Remove old declaration
		spec := oldInv.Type.Spec
		return &UpdatePlanActivity{
			Name: name,
			Type: ActivityRemoveOldDeclaration,
			Params: ActivityParams{
				Spec:            spec,
				SpecVersion:     string(spec),
				DeclarationFile: extractFilename(name),
			},
		}, nil

	case contains(name, "version directory"):
		// Version directory creation step
		versionPath := newInv.Head.String()
		return &UpdatePlanActivity{
			Name: name,
			Type: ActivityCreateVersionDir,
			Params: ActivityParams{
				VersionPath: versionPath,
			},
		}, nil

	case contains(name, "write "+newInv.Head.String()+"/"+inventoryBase):
		// Version inventory write
		return &UpdatePlanActivity{
			Name: name,
			Type: ActivityWriteVersionInventory,
			Params: ActivityParams{
				VersionPath:     newInv.Head.String(),
				InventoryJSON:   newInv.bytes,
				InventoryDigest: newInv.digest,
			},
		}, nil

	case contains(name, "write "+newInv.Head.String()+"/"+inventoryBase+"."):
		// Version sidecar write
		alg := newInv.DigestAlgorithm
		return &UpdatePlanActivity{
			Name: name,
			Type: ActivityWriteVersionSidecar,
			Params: ActivityParams{
				VersionPath:     newInv.Head.String(),
				SidecarPath:     newInv.Head.String() + "/" + inventoryBase + "." + alg,
				SidecarContent:  []byte(newInv.digest),
				DigestAlgorithm: alg,
			},
		}, nil

	case name == "write "+inventoryBase:
		// Root inventory write
		return &UpdatePlanActivity{
			Name: name,
			Type: ActivityWriteRootInventory,
			Params: ActivityParams{
				InventoryJSON:   newInv.bytes,
				InventoryDigest: newInv.digest,
			},
		}, nil

	case contains(name, "write "+inventoryBase+"."):
		// Root sidecar write
		alg := newInv.DigestAlgorithm
		return &UpdatePlanActivity{
			Name: name,
			Type: ActivityWriteRootSidecar,
			Params: ActivityParams{
				SidecarPath:     inventoryBase + "." + alg,
				SidecarContent:  []byte(newInv.digest),
				DigestAlgorithm: alg,
			},
		}, nil

	case name == "object root ":
		// Initial noop step - not a real activity
		return nil, nil

	default:
		// Unknown step type - return as generic activity
		return &UpdatePlanActivity{
			Name: name,
			Type: ActivityType(-1), // Unknown type
			Params: ActivityParams{},
		}, nil
	}
}

// ExecuteActivity executes a single UpdatePlanActivity.
// This function is designed to be called by durable execution framework activities.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - activity: The activity to execute
//   - objFS: The filesystem containing the OCFL object
//   - objPath: The path to the object root directory
//   - src: ContentSource for retrieving file content (required for ActivityCopyContent)
//
// Returns:
//   - ActivityResult with execution details
//   - Error if the activity failed
func ExecuteActivity(
	ctx context.Context,
	activity *UpdatePlanActivity,
	objFS ocflfs.FS,
	objPath string,
	src ContentSource,
) (ActivityResult, error) {
	if activity == nil {
		return ActivityResult{}, fmt.Errorf("activity is nil")
	}

	switch activity.Type {
	case ActivityDeclareObject:
		return executeDeclareObject(ctx, objFS, objPath, activity)

	case ActivityRemoveOldDeclaration:
		return executeRemoveDeclaration(ctx, objFS, objPath, activity)

	case ActivityCreateVersionDir:
		return executeCreateVersionDir(ctx, objFS, objPath, activity)

	case ActivityCopyContent:
		if src == nil {
			return ActivityResult{}, fmt.Errorf("content source is required for ActivityCopyContent")
		}
		return executeCopyContent(ctx, objFS, objPath, src, activity)

	case ActivityWriteVersionInventory:
		return executeWriteVersionInventory(ctx, objFS, objPath, activity)

	case ActivityWriteVersionSidecar:
		return executeWriteVersionSidecar(ctx, objFS, objPath, activity)

	case ActivityWriteRootInventory:
		return executeWriteRootInventory(ctx, objFS, objPath, activity)

	case ActivityWriteRootSidecar:
		return executeWriteRootSidecar(ctx, objFS, objPath, activity)

	default:
		return ActivityResult{}, fmt.Errorf("unknown activity type: %v", activity.Type)
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || find(s, substr) >= 0)
}

func find(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func extractDestPath(name string) string {
	// Extract path from "copy <path>" pattern
	prefix := "copy "
	if len(name) > len(prefix) && name[:len(prefix)] == prefix {
		return name[len(prefix):]
	}
	return ""
}

func extractFilename(name string) string {
	// Extract filename from "write <filename>" or "remove <filename>"
	for _, prefix := range []string{"write ", "remove "} {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			return name[len(prefix):]
		}
	}
	return ""
}
