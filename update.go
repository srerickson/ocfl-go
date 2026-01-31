package ocfl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// UpdatePlan represents a plan for updating an OCFL object.
// It contains a sequence of activities that must be executed to complete the update.
// This is the primary abstraction for durable execution frameworks.
type UpdatePlan struct {
	// ObjectID is the ID of the object being updated
	ObjectID string `json:"object_id"`

	// Activities is the ordered list of activities to execute
	Activities []*Activity `json:"activities"`

	// Metadata about the plan
	BaseVersion     VNum      `json:"base_version"`     // Version we're updating from (0 for new objects)
	TargetVersion   VNum      `json:"target_version"`   // Version we're creating
	Created         time.Time `json:"created"`          // When this plan was created
	Message         string    `json:"message"`          // Version message
	User            *User     `json:"user"`             // User creating the version
	DigestAlgorithm string    `json:"digest_algorithm"` // sha512 or sha256

	// Internal state (not serialized for frameworks)
	newInventory *Inventory `json:"-"`
	oldInventory *Inventory `json:"-"`
}

// Activity represents a single step in an OCFL object update.
// Each activity is independent, idempotent, and can be executed by
// durable execution frameworks.
type Activity struct {
	// Unique identifier for this activity (e.g., "copy-v1/content/file.txt")
	ID string `json:"id"`

	// Type of activity
	Type ActivityType `json:"type"`

	// Type-specific parameters
	Params ActivityParams `json:"params"`

	// Expected behavior
	ExpectedBytes int64 `json:"expected_bytes,omitempty"` // Expected bytes to write
	Idempotent    bool  `json:"idempotent"`               // Can be safely retried

	// Execution state (set by framework or executor)
	Status       ActivityStatus `json:"status,omitempty"`
	BytesWritten int64          `json:"bytes_written,omitempty"`
	Error        string         `json:"error,omitempty"`
	StartedAt    time.Time      `json:"started_at,omitempty"`
	CompletedAt  time.Time      `json:"completed_at,omitempty"`
}

// ActivityType identifies the type of activity
type ActivityType string

const (
	// Object-level activities
	ActivityTypeDeclareObject        ActivityType = "declare_object"
	ActivityTypeRemoveOldDeclaration ActivityType = "remove_old_declaration"

	// Version-level activities
	ActivityTypeCreateVersionDir      ActivityType = "create_version_dir"
	ActivityTypeCopyContent           ActivityType = "copy_content"
	ActivityTypeWriteVersionInventory ActivityType = "write_version_inventory"
	ActivityTypeWriteVersionSidecar   ActivityType = "write_version_sidecar"

	// Root-level activities
	ActivityTypeWriteRootInventory ActivityType = "write_root_inventory"
	ActivityTypeWriteRootSidecar   ActivityType = "write_root_sidecar"
)

// ActivityStatus represents the execution status of an activity
type ActivityStatus string

const (
	ActivityStatusPending   ActivityStatus = "pending"
	ActivityStatusRunning   ActivityStatus = "running"
	ActivityStatusCompleted ActivityStatus = "completed"
	ActivityStatusFailed    ActivityStatus = "failed"
	ActivityStatusSkipped   ActivityStatus = "skipped"
)

// ActivityParams contains type-specific parameters for activities
type ActivityParams struct {
	// For declare_object, remove_old_declaration
	Spec        Spec   `json:"spec,omitempty"`
	NamasteFile string `json:"namaste_file,omitempty"`

	// For create_version_dir
	VersionDir string `json:"version_dir,omitempty"`

	// For copy_content
	ContentDigest string `json:"content_digest,omitempty"`
	SourcePath    string `json:"source_path,omitempty"`
	DestPath      string `json:"dest_path,omitempty"`
	FileSize      int64  `json:"file_size,omitempty"`

	// For write_*_inventory
	InventoryJSON   []byte `json:"inventory_json,omitempty"`
	InventoryDigest string `json:"inventory_digest,omitempty"`
	InventoryPath   string `json:"inventory_path,omitempty"`

	// For write_*_sidecar
	SidecarPath    string `json:"sidecar_path,omitempty"`
	SidecarContent []byte `json:"sidecar_content,omitempty"`
}

// ActivityResult contains the result of executing an activity
type ActivityResult struct {
	Success      bool   `json:"success"`
	BytesWritten int64  `json:"bytes_written"`
	Skipped      bool   `json:"skipped"`
	SkipReason   string `json:"skip_reason,omitempty"`
	Error        error  `json:"-"`
	ErrorMessage string `json:"error,omitempty"`
}

// UpdatePlanBuilder creates update plans deterministically
type UpdatePlanBuilder struct {
	object          *Object
	stage           *Stage
	message         string
	user            *User
	timestamp       time.Time
	timestampSet    bool
	spec            Spec
	contentPathFunc PathMutation
}

// NewUpdatePlanBuilder creates a new builder for deterministic update plan creation
func (obj *Object) NewUpdatePlanBuilder(stage *Stage) *UpdatePlanBuilder {
	spec := Spec1_1 // default
	if obj.inventory != nil {
		spec = obj.inventory.Type.Spec
	}

	return &UpdatePlanBuilder{
		object: obj,
		stage:  stage,
		spec:   spec,
	}
}

// WithTimestamp sets the version creation timestamp (REQUIRED for determinism)
func (b *UpdatePlanBuilder) WithTimestamp(t time.Time) *UpdatePlanBuilder {
	b.timestamp = t
	b.timestampSet = true
	return b
}

// WithMessage sets the version message
func (b *UpdatePlanBuilder) WithMessage(msg string) *UpdatePlanBuilder {
	b.message = msg
	return b
}

// WithUser sets the user creating the version
func (b *UpdatePlanBuilder) WithUser(user *User) *UpdatePlanBuilder {
	b.user = user
	return b
}

// WithSpec sets the OCFL spec version
func (b *UpdatePlanBuilder) WithSpec(spec Spec) *UpdatePlanBuilder {
	b.spec = spec
	return b
}

// WithContentPathFunc sets custom content path generation
func (b *UpdatePlanBuilder) WithContentPathFunc(fn PathMutation) *UpdatePlanBuilder {
	b.contentPathFunc = fn
	return b
}

// Build creates the update plan
func (b *UpdatePlanBuilder) Build() (*UpdatePlan, error) {
	if !b.timestampSet {
		return nil, fmt.Errorf("timestamp must be set via WithTimestamp() for deterministic plan creation")
	}

	if b.user == nil {
		return nil, fmt.Errorf("user must be set via WithUser()")
	}

	if b.message == "" {
		return nil, fmt.Errorf("message must be set via WithMessage()")
	}

	// Build new inventory
	invBuilder := b.object.InventoryBuilder().
		Spec(b.spec).
		AddVersion(b.stage.State, b.stage.DigestAlgorithm, b.timestamp, b.message, b.user)

	if b.contentPathFunc != nil {
		invBuilder = invBuilder.ContentPathFunc(b.contentPathFunc)
	}

	// Preserve fixity information from stage
	if b.stage.FixitySource != nil {
		invBuilder = invBuilder.FixitySource(b.stage.FixitySource)
	}

	newInv, err := invBuilder.Finalize()
	if err != nil {
		return nil, fmt.Errorf("building inventory: %w", err)
	}

	// Create plan
	plan := &UpdatePlan{
		ObjectID:        b.object.ID(),
		Created:         b.timestamp,
		Message:         b.message,
		User:            b.user,
		DigestAlgorithm: b.stage.DigestAlgorithm.ID(),
		newInventory:    newInv,
		TargetVersion:   newInv.Head,
	}

	if b.object.inventory != nil {
		plan.oldInventory = &b.object.inventory.Inventory
		plan.BaseVersion = b.object.inventory.Head
	}

	// Generate activities
	if err := plan.generateActivities(); err != nil {
		return nil, fmt.Errorf("generating activities: %w", err)
	}

	return plan, nil
}

// generateActivities creates the activity list from inventory changes
func (plan *UpdatePlan) generateActivities() error {
	var activities []*Activity

	versionDir := plan.TargetVersion.String()
	isFirstVersion := plan.BaseVersion.Num() == 0

	// 1. Declare object (first version only)
	if isFirstVersion {
		activities = append(activities, &Activity{
			ID:         "declare-object",
			Type:       ActivityTypeDeclareObject,
			Idempotent: true,
			Params: ActivityParams{
				Spec:        plan.newInventory.Type.Spec,
				NamasteFile: fmt.Sprintf("0=ocfl_object_%s", plan.newInventory.Type.Spec),
			},
		})
	}

	// 2. Remove old declaration if spec changed
	if !isFirstVersion && plan.oldInventory != nil {
		oldSpec := plan.oldInventory.Type.Spec
		newSpec := plan.newInventory.Type.Spec
		if oldSpec != newSpec {
			activities = append(activities, &Activity{
				ID:         "remove-old-declaration",
				Type:       ActivityTypeRemoveOldDeclaration,
				Idempotent: true,
				Params: ActivityParams{
					Spec:        oldSpec,
					NamasteFile: fmt.Sprintf("0=ocfl_object_%s", oldSpec),
				},
			})

			// Write new declaration
			activities = append(activities, &Activity{
				ID:         "write-new-declaration",
				Type:       ActivityTypeDeclareObject,
				Idempotent: true,
				Params: ActivityParams{
					Spec:        newSpec,
					NamasteFile: fmt.Sprintf("0=ocfl_object_%s", newSpec),
				},
			})
		}
	}

	// 3. Create version directory (marker activity - actual dir created with first file)
	activities = append(activities, &Activity{
		ID:         fmt.Sprintf("create-version-%s", versionDir),
		Type:       ActivityTypeCreateVersionDir,
		Idempotent: true,
		Params: ActivityParams{
			VersionDir: versionDir,
		},
	})

	// 4. Copy content files
	newContent := plan.newInventory.versionContent(plan.TargetVersion)
	for destPath, contentDigest := range newContent.SortedPaths() {
		// Skip if content already exists in object
		if plan.oldInventory != nil {
			if _, exists := plan.oldInventory.Manifest[contentDigest]; exists {
				continue // Content already in object, no need to copy
			}
		}

		activities = append(activities, &Activity{
			ID:         fmt.Sprintf("copy-%s", destPath),
			Type:       ActivityTypeCopyContent,
			Idempotent: true,
			Params: ActivityParams{
				ContentDigest: contentDigest,
				DestPath:      destPath,
			},
		})
	}

	// 5. Write version inventory
	invBytes, err := json.Marshal(plan.newInventory)
	if err != nil {
		return fmt.Errorf("marshaling inventory: %w", err)
	}

	// Get digest algorithm and compute inventory digest
	alg, err := digest.DefaultRegistry().NewDigester(plan.newInventory.DigestAlgorithm)
	if err != nil {
		return fmt.Errorf("creating digester: %w", err)
	}
	if _, err := alg.Write(invBytes); err != nil {
		return fmt.Errorf("computing inventory digest: %w", err)
	}
	invDigest := alg.String()
	invPath := fmt.Sprintf("%s/inventory.json", versionDir)

	activities = append(activities, &Activity{
		ID:            fmt.Sprintf("write-version-inventory-%s", versionDir),
		Type:          ActivityTypeWriteVersionInventory,
		Idempotent:    true,
		ExpectedBytes: int64(len(invBytes)),
		Params: ActivityParams{
			InventoryJSON:   invBytes,
			InventoryDigest: invDigest,
			InventoryPath:   invPath,
		},
	})

	// 6. Write version sidecar
	sidecarPath := fmt.Sprintf("%s/inventory.json.%s", versionDir, plan.DigestAlgorithm)
	sidecarContent := []byte(invDigest + " " + inventoryBase + "\n")

	activities = append(activities, &Activity{
		ID:            fmt.Sprintf("write-version-sidecar-%s", versionDir),
		Type:          ActivityTypeWriteVersionSidecar,
		Idempotent:    true,
		ExpectedBytes: int64(len(sidecarContent)),
		Params: ActivityParams{
			SidecarPath:    sidecarPath,
			SidecarContent: sidecarContent,
		},
	})

	// 7. Write root inventory
	activities = append(activities, &Activity{
		ID:            "write-root-inventory",
		Type:          ActivityTypeWriteRootInventory,
		Idempotent:    false, // Root inventory changes object state
		ExpectedBytes: int64(len(invBytes)),
		Params: ActivityParams{
			InventoryJSON:   invBytes,
			InventoryDigest: invDigest,
			InventoryPath:   "inventory.json",
		},
	})

	// 8. Write root sidecar
	rootSidecarPath := fmt.Sprintf("inventory.json.%s", plan.DigestAlgorithm)
	activities = append(activities, &Activity{
		ID:            "write-root-sidecar",
		Type:          ActivityTypeWriteRootSidecar,
		Idempotent:    false,
		ExpectedBytes: int64(len(sidecarContent)),
		Params: ActivityParams{
			SidecarPath:    rootSidecarPath,
			SidecarContent: sidecarContent,
		},
	})

	plan.Activities = activities
	return nil
}

// Execute runs a single activity
func (a *Activity) Execute(ctx context.Context, objFS ocflfs.FS, objPath string, src ContentSource) (ActivityResult, error) {
	a.Status = ActivityStatusRunning
	a.StartedAt = time.Now()

	var result ActivityResult

	switch a.Type {
	case ActivityTypeDeclareObject:
		result = executeDeclareObject(ctx, objFS, objPath, a)
	case ActivityTypeRemoveOldDeclaration:
		result = executeRemoveDeclaration(ctx, objFS, objPath, a)
	case ActivityTypeCreateVersionDir:
		result = executeCreateVersionDir(ctx, objFS, objPath, a)
	case ActivityTypeCopyContent:
		result = executeCopyContent(ctx, objFS, objPath, src, a)
	case ActivityTypeWriteVersionInventory:
		result = executeWriteInventory(ctx, objFS, objPath, a, true)
	case ActivityTypeWriteVersionSidecar:
		result = executeWriteSidecar(ctx, objFS, objPath, a, true)
	case ActivityTypeWriteRootInventory:
		result = executeWriteInventory(ctx, objFS, objPath, a, false)
	case ActivityTypeWriteRootSidecar:
		result = executeWriteSidecar(ctx, objFS, objPath, a, false)
	default:
		result = ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("unknown activity type: %s", a.Type),
			ErrorMessage: fmt.Sprintf("unknown activity type: %s", a.Type),
		}
	}

	a.CompletedAt = time.Now()
	a.BytesWritten = result.BytesWritten

	if result.Success {
		if result.Skipped {
			a.Status = ActivityStatusSkipped
		} else {
			a.Status = ActivityStatusCompleted
		}
	} else {
		a.Status = ActivityStatusFailed
		a.Error = result.ErrorMessage
	}

	return result, result.Error
}

func executeDeclareObject(ctx context.Context, objFS ocflfs.FS, objPath string, activity *Activity) ActivityResult {
	spec := activity.Params.Spec
	if spec.Empty() {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("spec not specified in activity"),
			ErrorMessage: "spec not specified in activity",
		}
	}

	namaste := Namaste{
		Type:    NamasteTypeObject,
		Version: spec,
	}

	// Check if declaration already exists (idempotency)
	declPath := path.Join(objPath, namaste.Name())
	if _, err := ocflfs.StatFile(ctx, objFS, declPath); err == nil {
		return ActivityResult{
			Success:    true,
			Skipped:    true,
			SkipReason: "declaration file already exists",
		}
	}

	// Write the declaration
	if err := WriteDeclaration(ctx, objFS, objPath, namaste); err != nil {
		return ActivityResult{
			Success:      false,
			Error:        err,
			ErrorMessage: err.Error(),
		}
	}

	return ActivityResult{
		Success:      true,
		BytesWritten: int64(len(namaste.Name())),
	}
}

func executeRemoveDeclaration(ctx context.Context, objFS ocflfs.FS, objPath string, activity *Activity) ActivityResult {
	spec := activity.Params.Spec
	if spec.Empty() {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("spec not specified"),
			ErrorMessage: "spec not specified",
		}
	}

	namaste := Namaste{
		Type:    NamasteTypeObject,
		Version: spec,
	}

	declPath := path.Join(objPath, namaste.Name())

	// Remove the declaration (idempotent - doesn't fail if not exists)
	err := ocflfs.Remove(ctx, objFS, declPath)
	if errors.Is(err, fs.ErrNotExist) {
		return ActivityResult{
			Success:    true,
			Skipped:    true,
			SkipReason: "declaration file does not exist",
		}
	}

	if err != nil {
		return ActivityResult{
			Success:      false,
			Error:        err,
			ErrorMessage: err.Error(),
		}
	}

	return ActivityResult{
		Success: true,
	}
}

func executeCreateVersionDir(ctx context.Context, objFS ocflfs.FS, objPath string, activity *Activity) ActivityResult {
	versionDir := activity.Params.VersionDir
	if versionDir == "" {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("version directory not specified"),
			ErrorMessage: "version directory not specified",
		}
	}

	fullPath := path.Join(objPath, versionDir)

	// Check if directory already exists
	if stat, err := ocflfs.StatFile(ctx, objFS, fullPath); err == nil {
		if !stat.IsDir() {
			return ActivityResult{
				Success:      false,
				Error:        fmt.Errorf("version path exists but is not a directory"),
				ErrorMessage: "version path exists but is not a directory",
			}
		}
		return ActivityResult{
			Success:    true,
			Skipped:    true,
			SkipReason: "version directory already exists",
		}
	}

	// Directory will be created automatically when files are written
	// This is just a marker activity
	return ActivityResult{
		Success: true,
	}
}

func executeCopyContent(ctx context.Context, objFS ocflfs.FS, objPath string, src ContentSource, activity *Activity) ActivityResult {
	contentDigest := activity.Params.ContentDigest
	destPath := activity.Params.DestPath

	if contentDigest == "" {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("content digest not specified"),
			ErrorMessage: "content digest not specified",
		}
	}

	if destPath == "" {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("destination path not specified"),
			ErrorMessage: "destination path not specified",
		}
	}

	// Get content from source
	if src == nil {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("content source is nil"),
			ErrorMessage: "content source is nil",
		}
	}

	srcFS, srcPath := src.GetContent(contentDigest)
	if srcFS == nil {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("content source doesn't provide digest %q", contentDigest),
			ErrorMessage: fmt.Sprintf("content source doesn't provide digest %q", contentDigest),
		}
	}

	fullDestPath := path.Join(objPath, destPath)

	// Check if file already exists with correct digest (idempotency)
	if stat, err := ocflfs.StatFile(ctx, objFS, fullDestPath); err == nil {
		// File exists - verify it has the correct content
		if err := verifyFileDigest(ctx, objFS, fullDestPath, contentDigest); err == nil {
			// File exists and has correct content
			return ActivityResult{
				Success:      true,
				BytesWritten: stat.Size(),
				Skipped:      true,
				SkipReason:   "file already exists with correct content",
			}
		}
		// File exists but has wrong content - error!
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("file exists at %q but has incorrect digest", fullDestPath),
			ErrorMessage: fmt.Sprintf("file exists at %q but has incorrect digest", fullDestPath),
		}
	}

	// Copy the file
	bytesWritten, err := ocflfs.Copy(ctx, objFS, fullDestPath, srcFS, srcPath)
	if err != nil {
		return ActivityResult{
			Success:      false,
			Error:        err,
			ErrorMessage: err.Error(),
		}
	}

	// Verify the copied content
	if err := verifyFileDigest(ctx, objFS, fullDestPath, contentDigest); err != nil {
		// Verification failed - remove the bad file
		_ = ocflfs.Remove(ctx, objFS, fullDestPath)
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("digest verification failed after copy: %w", err),
			ErrorMessage: fmt.Sprintf("digest verification failed after copy: %v", err),
		}
	}

	return ActivityResult{
		Success:      true,
		BytesWritten: bytesWritten,
	}
}

func executeWriteInventory(ctx context.Context, objFS ocflfs.FS, objPath string, activity *Activity, isVersion bool) ActivityResult {
	invJSON := activity.Params.InventoryJSON
	invDigest := activity.Params.InventoryDigest
	invPath := activity.Params.InventoryPath

	if len(invJSON) == 0 {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("inventory JSON not specified"),
			ErrorMessage: "inventory JSON not specified",
		}
	}

	if invPath == "" {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("inventory path not specified"),
			ErrorMessage: "inventory path not specified",
		}
	}

	fullPath := path.Join(objPath, invPath)

	// For version inventories, check idempotency
	if isVersion {
		if _, err := ocflfs.StatFile(ctx, objFS, fullPath); err == nil {
			if err := verifyFileDigest(ctx, objFS, fullPath, invDigest); err == nil {
				return ActivityResult{
					Success:      true,
					BytesWritten: int64(len(invJSON)),
					Skipped:      true,
					SkipReason:   "inventory already exists with correct content",
				}
			}
		}
	}

	// Write the inventory
	bytesWritten, err := ocflfs.Write(ctx, objFS, fullPath, bytes.NewReader(invJSON))
	if err != nil {
		return ActivityResult{
			Success:      false,
			Error:        err,
			ErrorMessage: err.Error(),
		}
	}

	// Verify written content
	if err := verifyFileDigest(ctx, objFS, fullPath, invDigest); err != nil {
		_ = ocflfs.Remove(ctx, objFS, fullPath)
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("digest verification failed after write: %w", err),
			ErrorMessage: fmt.Sprintf("digest verification failed after write: %v", err),
		}
	}

	return ActivityResult{
		Success:      true,
		BytesWritten: bytesWritten,
	}
}

func executeWriteSidecar(ctx context.Context, objFS ocflfs.FS, objPath string, activity *Activity, isVersion bool) ActivityResult {
	sidecarPath := activity.Params.SidecarPath
	sidecarContent := activity.Params.SidecarContent

	if sidecarPath == "" {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("sidecar path not specified"),
			ErrorMessage: "sidecar path not specified",
		}
	}

	if len(sidecarContent) == 0 {
		return ActivityResult{
			Success:      false,
			Error:        fmt.Errorf("sidecar content not specified"),
			ErrorMessage: "sidecar content not specified",
		}
	}

	fullPath := path.Join(objPath, sidecarPath)

	// For version sidecars, check idempotency
	if isVersion {
		if existing, err := ocflfs.ReadAll(ctx, objFS, fullPath); err == nil {
			if bytes.Equal(existing, sidecarContent) {
				return ActivityResult{
					Success:      true,
					BytesWritten: int64(len(sidecarContent)),
					Skipped:      true,
					SkipReason:   "sidecar already exists with correct content",
				}
			}
		}
	}

	// Write the sidecar
	bytesWritten, err := ocflfs.Write(ctx, objFS, fullPath, bytes.NewReader(sidecarContent))
	if err != nil {
		return ActivityResult{
			Success:      false,
			Error:        err,
			ErrorMessage: err.Error(),
		}
	}

	return ActivityResult{
		Success:      true,
		BytesWritten: bytesWritten,
	}
}

// Helper function to verify file digest
func verifyFileDigest(ctx context.Context, fsys ocflfs.FS, filePath, expectedDigest string) error {
	// Read the file content
	content, err := ocflfs.ReadAll(ctx, fsys, filePath)
	if err != nil {
		return err
	}

	// Try both SHA512 and SHA256
	for _, alg := range []digest.Algorithm{digest.SHA512, digest.SHA256} {
		digester := alg.Digester()
		_, err := digester.Write(content)
		if err != nil {
			continue
		}

		computedDigest := digester.String()
		if computedDigest == expectedDigest {
			return nil // Match found
		}
	}

	return fmt.Errorf("digest mismatch: expected %s, but file content doesn't match", expectedDigest)
}
