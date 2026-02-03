package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"log"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	awsS3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/sqlite"
	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/core"
	"github.com/cschleiden/go-workflows/worker"
	"github.com/cschleiden/go-workflows/workflow"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/logging"
)

// cmdFlags mirrors the update example's command line flags
type cmdFlags struct {
	objPath string // path to object
	srcDir  string // path to content directory
	msg     string // message for new version
	algID   string // digest algorithm (sha512 or sha256)
	newID   string // ID for new object
	user    ocfl.User
	dbPath  string // workflow database path
	resume  bool   // resume an interrupted workflow
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	f, err := parseArgs()
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	// Initialize storage backend
	writeFS, objDir, err := parseStoreConn(ctx, f.objPath)
	if err != nil {
		return fmt.Errorf("parsing object path: %w", err)
	}

	// Determine workflow instance ID
	instanceID := f.newID
	if instanceID == "" {
		instanceID = f.objPath
	}

	// Initialize workflow backend
	b := sqlite.NewSqliteBackend(f.dbPath)

	// Create worker/orchestrator
	orchestrator := worker.NewWorkflowOrchestrator(b, nil)
	orchestrator.RegisterWorkflow(UpdateObjectWorkflow)
	orchestrator.RegisterActivity(&ocflActivities{fsys: writeFS, objDir: objDir})

	// Create a cancellable context for the orchestrator
	orchCtx, cancelOrch := context.WithCancel(ctx)
	defer func() {
		cancelOrch()
		orchestrator.WaitForCompletion()
		b.Close()
	}()

	if err := orchestrator.Start(orchCtx); err != nil {
		return fmt.Errorf("starting orchestrator: %w", err)
	}

	// Handle resume mode
	if f.resume {
		return resumeWorkflow(ctx, orchestrator, instanceID)
	}

	// Get digest algorithm
	alg, err := digest.DefaultRegistry().Get(f.algID)
	if err != nil {
		return fmt.Errorf("getting digest algorithm: %w", err)
	}

	// Stage source content before starting workflow
	stage, err := ocfl.StageDir(ctx, ocflfs.DirFS(f.srcDir), ".", alg)
	if err != nil {
		return fmt.Errorf("staging content: %w", err)
	}

	// Extract serializable content map
	contentMap := extractContentMap(stage)

	// Create workflow input
	input := UpdateWorkflowInput{
		ObjDir:          objDir,
		SrcDir:          f.srcDir,
		ContentMap:      contentMap,
		State:           stage.State,
		DigestAlgorithm: f.algID,
		Message:         f.msg,
		UserName:        f.user.Name,
		UserAddress:     f.user.Address,
		NewID:           f.newID,
	}

	// Start workflow
	wfOptions := client.WorkflowInstanceOptions{
		InstanceID: instanceID,
	}
	wf, err := orchestrator.CreateWorkflowInstance(ctx, wfOptions,
		UpdateObjectWorkflow, input)
	if err != nil {
		// If workflow already exists, offer to resume it
		if errors.Is(err, backend.ErrInstanceAlreadyExists) {
			return fmt.Errorf("workflow %q already exists; use -resume to continue it", instanceID)
		}
		return fmt.Errorf("creating workflow: %w", err)
	}

	// Wait for result
	result, err := client.GetWorkflowResult[string](ctx, orchestrator.Client,
		wf, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("workflow failed: %w", err)
	}

	fmt.Printf("Update completed: %s\n", result)
	return nil
}

// resumeWorkflow resumes an interrupted workflow and waits for its completion
func resumeWorkflow(ctx context.Context, orchestrator *worker.WorkflowOrchestrator,
	instanceID string) error {

	// Create a workflow instance reference with just the instance ID.
	// The execution ID is not strictly required for waiting.
	wf := core.NewWorkflowInstance(instanceID, "")

	// Check if the workflow exists and get its state
	state, err := orchestrator.Client.GetWorkflowInstanceState(ctx, wf)
	if err != nil {
		if errors.Is(err, backend.ErrInstanceNotFound) {
			return fmt.Errorf("workflow %q not found (may have completed already)", instanceID)
		}
		return fmt.Errorf("checking workflow state: %w", err)
	}

	switch state {
	case core.WorkflowInstanceStateFinished:
		fmt.Printf("Workflow %q already completed\n", instanceID)
		return nil
	case core.WorkflowInstanceStateActive:
		fmt.Printf("Resuming workflow %q...\n", instanceID)
	default:
		fmt.Printf("Resuming workflow %q (state: %v)...\n", instanceID, state)
	}

	// Wait for the workflow to complete
	result, err := client.GetWorkflowResult[string](ctx, orchestrator.Client,
		wf, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("workflow failed: %w", err)
	}

	fmt.Printf("Update completed: %s\n", result)
	return nil
}

func parseArgs() (*cmdFlags, error) {
	var f cmdFlags
	flag.StringVar(&f.objPath, "obj", "", "path to OCFL object to create/update")
	flag.StringVar(&f.srcDir, "src", "", "local path with new object content")
	flag.StringVar(&f.msg, "msg", "", "message field for new version")
	flag.StringVar(&f.user.Name, "name", "", "name field for new version")
	flag.StringVar(&f.user.Address, "email", "", "email field for new version")
	flag.StringVar(&f.algID, "alg", "sha512", "digest algorithm for new version")
	flag.StringVar(&f.newID, "id", "", "object ID (required for new objects)")
	flag.StringVar(&f.dbPath, "db", "workflow.db", "workflow database path")
	flag.BoolVar(&f.resume, "resume", false, "resume an interrupted workflow")
	flag.Parse()

	// Validate required flags based on mode
	if f.resume {
		// Resume mode only requires -obj (or -id)
		if f.objPath == "" && f.newID == "" {
			return nil, errors.New("resume requires -obj or -id flag")
		}
	} else {
		// Normal mode requires -obj, -src, -msg, -name, -email
		var missing []string
		flag.VisitAll(func(fl *flag.Flag) {
			if fl.Name == "id" || fl.Name == "alg" || fl.Name == "db" || fl.Name == "resume" {
				return
			}
			if v := fl.Value.String(); v == "" {
				missing = append(missing, fl.Name)
			}
		})
		if len(missing) > 0 {
			return nil, errors.New("missing required flags: " + strings.Join(missing, ", "))
		}
	}
	return &f, nil
}

// extractContentMap builds a mapping from digest to source file path
func extractContentMap(stage *ocfl.Stage) map[string]string {
	cm := make(map[string]string)
	for dig, paths := range stage.State {
		if len(paths) > 0 {
			// Use the first path for this digest
			cm[dig] = paths[0]
		}
	}
	return cm
}

// UpdateWorkflowInput contains all serializable data for the workflow
type UpdateWorkflowInput struct {
	ObjDir          string              `json:"obj_dir"`
	SrcDir          string              `json:"src_dir"`
	ContentMap      map[string]string   `json:"content_map"`
	State           map[string][]string `json:"state"`
	DigestAlgorithm string              `json:"digest_algorithm"`
	Message         string              `json:"message"`
	UserName        string              `json:"user_name"`
	UserAddress     string              `json:"user_address"`
	NewID           string              `json:"new_id"`
}

// UpdateObjectWorkflow implements the durable update workflow
func UpdateObjectWorkflow(ctx workflow.Context, input UpdateWorkflowInput) (string, error) {
	opts := workflow.ActivityOptions{
		RetryOptions: workflow.RetryOptions{
			MaxAttempts: 3,
		},
	}
	var acts *ocflActivities

	// Step 1: Prepare update (build inventory)
	prepInput := PrepareUpdateInput{
		State:           input.State,
		DigestAlgorithm: input.DigestAlgorithm,
		Message:         input.Message,
		UserName:        input.UserName,
		UserAddress:     input.UserAddress,
		NewID:           input.NewID,
	}
	prep, err := workflow.ExecuteActivity[*PrepareUpdateResult](
		ctx, opts, acts.PrepareUpdate, prepInput).Get(ctx)
	if err != nil {
		return "", fmt.Errorf("prepare update: %w", err)
	}

	// Step 2: Write declaration if needed (new object or spec change)
	if prep.NewSpec != prep.OldSpec {
		declInput := WriteDeclarationInput{
			NewSpec: prep.NewSpec,
			OldSpec: prep.OldSpec,
		}
		_, err := workflow.ExecuteActivity[bool](
			ctx, opts, acts.WriteDeclaration, declInput).Get(ctx)
		if err != nil {
			return "", fmt.Errorf("write declaration: %w", err)
		}
	}

	// Step 3: Copy content files (in sorted order for determinism)
	dstPaths := make([]string, 0, len(prep.ContentToCopy))
	for dstPath := range prep.ContentToCopy {
		dstPaths = append(dstPaths, dstPath)
	}
	slices.Sort(dstPaths)

	for _, dstPath := range dstPaths {
		dig := prep.ContentToCopy[dstPath]
		srcPath := input.ContentMap[dig]
		copyInput := CopyContentInput{
			SrcDir:  input.SrcDir,
			SrcPath: srcPath,
			DstPath: dstPath,
			Digest:  dig,
		}
		_, err := workflow.ExecuteActivity[int64](
			ctx, opts, acts.CopyContent, copyInput).Get(ctx)
		if err != nil {
			return "", fmt.Errorf("copy content %s: %w", dstPath, err)
		}
	}

	// Step 4: Write version inventory
	verInvInput := WriteInventoryInput{
		VersionDir:      prep.NewHead,
		InventoryBytes:  prep.InventoryBytes,
		InventoryDigest: prep.InventoryDigest,
		DigestAlgorithm: input.DigestAlgorithm,
	}
	_, err = workflow.ExecuteActivity[bool](
		ctx, opts, acts.WriteVersionInventory, verInvInput).Get(ctx)
	if err != nil {
		return "", fmt.Errorf("write version inventory: %w", err)
	}

	// Step 5: Write root inventory
	rootInvInput := WriteRootInventoryInput{
		InventoryBytes:     prep.InventoryBytes,
		InventoryDigest:    prep.InventoryDigest,
		DigestAlgorithm:    input.DigestAlgorithm,
		OldDigestAlgorithm: prep.OldDigestAlgorithm,
	}
	_, err = workflow.ExecuteActivity[bool](
		ctx, opts, acts.WriteRootInventory, rootInvInput).Get(ctx)
	if err != nil {
		return "", fmt.Errorf("write root inventory: %w", err)
	}

	return prep.NewHead, nil
}

// ocflActivities holds the filesystem for activity methods
type ocflActivities struct {
	fsys   ocflfs.FS
	objDir string
}

// PrepareUpdateInput contains data for preparing the update
type PrepareUpdateInput struct {
	State           map[string][]string `json:"state"`
	DigestAlgorithm string              `json:"digest_algorithm"`
	Message         string              `json:"message"`
	UserName        string              `json:"user_name"`
	UserAddress     string              `json:"user_address"`
	NewID           string              `json:"new_id"`
}

// PrepareUpdateResult contains the inventory and content info
type PrepareUpdateResult struct {
	InventoryBytes     []byte            `json:"inventory_bytes"`
	InventoryDigest    string            `json:"inventory_digest"`
	NewHead            string            `json:"new_head"`
	ContentToCopy      map[string]string `json:"content_to_copy"`
	NewSpec            string            `json:"new_spec"`
	OldSpec            string            `json:"old_spec"`
	OldDigestAlgorithm string            `json:"old_digest_algorithm"`
}

// PrepareUpdate builds the new inventory and determines what content to copy
func (a *ocflActivities) PrepareUpdate(ctx context.Context,
	input PrepareUpdateInput) (*PrepareUpdateResult, error) {

	// Open or create the object
	var objOpts []ocfl.ObjectOption
	if input.NewID != "" {
		objOpts = append(objOpts, ocfl.ObjectWithID(input.NewID))
	}
	obj, err := ocfl.NewObject(ctx, a.fsys, a.objDir, objOpts...)
	if err != nil {
		return nil, fmt.Errorf("opening object: %w", err)
	}

	// Check if new object needs ID
	if !obj.Exists() && input.NewID == "" {
		return nil, errors.New("'id' flag required for new objects")
	}

	// Get digest algorithm
	alg, err := digest.DefaultRegistry().Get(input.DigestAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("getting algorithm: %w", err)
	}

	// Convert state to DigestMap
	state := make(ocfl.DigestMap)
	for dig, paths := range input.State {
		state[dig] = paths
	}

	// Build new inventory
	builder := obj.InventoryBuilder()
	builder.AddVersion(state, alg, time.Now(), input.Message,
		&ocfl.User{Name: input.UserName, Address: input.UserAddress})

	newInv, err := builder.Finalize()
	if err != nil {
		return nil, fmt.Errorf("building inventory: %w", err)
	}

	// Marshal inventory to JSON
	invBytes, err := json.Marshal(newInv)
	if err != nil {
		return nil, fmt.Errorf("marshaling inventory: %w", err)
	}

	// Compute inventory digest
	invDigest, err := computeDigest(invBytes, input.DigestAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("computing inventory digest: %w", err)
	}

	// Determine content to copy (files in new version directory)
	prefix := newInv.Head.String() + "/"
	contentToCopy := make(map[string]string)
	for pth, dig := range newInv.Manifest.Paths() {
		if strings.HasPrefix(pth, prefix) {
			contentToCopy[pth] = dig
		}
	}

	// Get old spec info
	var oldSpec, oldAlg string
	if obj.Exists() {
		oldSpec = string(obj.Spec())
		oldAlg = obj.DigestAlgorithm().ID()
	}

	return &PrepareUpdateResult{
		InventoryBytes:     invBytes,
		InventoryDigest:    invDigest,
		NewHead:            newInv.Head.String(),
		ContentToCopy:      contentToCopy,
		NewSpec:            string(newInv.Type.Spec),
		OldSpec:            oldSpec,
		OldDigestAlgorithm: oldAlg,
	}, nil
}

// WriteDeclarationInput contains data for writing the NAMASTE declaration
type WriteDeclarationInput struct {
	NewSpec string `json:"new_spec"`
	OldSpec string `json:"old_spec"`
}

// WriteDeclaration writes the NAMASTE declaration file
func (a *ocflActivities) WriteDeclaration(ctx context.Context,
	input WriteDeclarationInput) (bool, error) {

	// Write new declaration
	newSpec := ocfl.Spec(input.NewSpec)
	newDecl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: newSpec}
	if err := ocfl.WriteDeclaration(ctx, a.fsys, a.objDir, newDecl); err != nil {
		return false, fmt.Errorf("writing declaration: %w", err)
	}

	// Remove old declaration if different
	if input.OldSpec != "" && input.OldSpec != input.NewSpec {
		oldSpec := ocfl.Spec(input.OldSpec)
		oldDecl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: oldSpec}
		oldPath := path.Join(a.objDir, oldDecl.Name())
		if err := ocflfs.Remove(ctx, a.fsys, oldPath); err != nil {
			// Ignore if not found
			if !errors.Is(err, fs.ErrNotExist) {
				return false, fmt.Errorf("removing old declaration: %w", err)
			}
		}
	}

	return true, nil
}

// CopyContentInput contains data for copying a content file
type CopyContentInput struct {
	SrcDir  string `json:"src_dir"`
	SrcPath string `json:"src_path"`
	DstPath string `json:"dst_path"`
	Digest  string `json:"digest"`
}

// CopyContent copies a single content file from staging to the object
func (a *ocflActivities) CopyContent(ctx context.Context,
	input CopyContentInput) (int64, error) {

	// Create source filesystem
	srcFS, err := local.NewFS(input.SrcDir)
	if err != nil {
		return 0, fmt.Errorf("opening source dir: %w", err)
	}

	// Copy file
	dstPath := path.Join(a.objDir, input.DstPath)
	size, err := ocflfs.Copy(ctx, a.fsys, dstPath, srcFS, input.SrcPath)
	if err != nil {
		return 0, fmt.Errorf("copying file: %w", err)
	}

	return size, nil
}

// WriteInventoryInput contains data for writing an inventory file
type WriteInventoryInput struct {
	VersionDir      string `json:"version_dir"`
	InventoryBytes  []byte `json:"inventory_bytes"`
	InventoryDigest string `json:"inventory_digest"`
	DigestAlgorithm string `json:"digest_algorithm"`
}

// WriteVersionInventory writes the inventory to the version directory
func (a *ocflActivities) WriteVersionInventory(ctx context.Context,
	input WriteInventoryInput) (bool, error) {

	verDir := path.Join(a.objDir, input.VersionDir)

	// Write inventory.json
	invPath := path.Join(verDir, "inventory.json")
	_, err := ocflfs.Write(ctx, a.fsys, invPath, bytes.NewReader(input.InventoryBytes))
	if err != nil {
		return false, fmt.Errorf("writing inventory: %w", err)
	}

	// Write sidecar
	if err := writeSidecar(ctx, a.fsys, verDir,
		input.InventoryDigest, input.DigestAlgorithm); err != nil {
		return false, fmt.Errorf("writing sidecar: %w", err)
	}

	return true, nil
}

// WriteRootInventoryInput contains data for writing the root inventory
type WriteRootInventoryInput struct {
	InventoryBytes     []byte `json:"inventory_bytes"`
	InventoryDigest    string `json:"inventory_digest"`
	DigestAlgorithm    string `json:"digest_algorithm"`
	OldDigestAlgorithm string `json:"old_digest_algorithm"`
}

// WriteRootInventory writes the inventory to the object root
func (a *ocflActivities) WriteRootInventory(ctx context.Context,
	input WriteRootInventoryInput) (bool, error) {

	// Write inventory.json
	invPath := path.Join(a.objDir, "inventory.json")
	_, err := ocflfs.Write(ctx, a.fsys, invPath, bytes.NewReader(input.InventoryBytes))
	if err != nil {
		return false, fmt.Errorf("writing inventory: %w", err)
	}

	// Write sidecar
	if err := writeSidecar(ctx, a.fsys, a.objDir,
		input.InventoryDigest, input.DigestAlgorithm); err != nil {
		return false, fmt.Errorf("writing sidecar: %w", err)
	}

	// Remove old sidecar if algorithm changed
	if input.OldDigestAlgorithm != "" &&
		input.OldDigestAlgorithm != input.DigestAlgorithm {
		oldSidecar := path.Join(a.objDir, "inventory.json."+input.OldDigestAlgorithm)
		if err := ocflfs.Remove(ctx, a.fsys, oldSidecar); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return false, fmt.Errorf("removing old sidecar: %w", err)
			}
		}
	}

	return true, nil
}

// writeSidecar writes an inventory sidecar file
func writeSidecar(ctx context.Context, fsys ocflfs.FS, dir, invDigest, alg string) error {
	content := invDigest + " inventory.json\n"
	sidecarPath := path.Join(dir, "inventory.json."+alg)
	_, err := ocflfs.Write(ctx, fsys, sidecarPath, strings.NewReader(content))
	return err
}

// computeDigest computes a digest of the given bytes
func computeDigest(data []byte, alg string) (string, error) {
	var h hash.Hash
	switch alg {
	case "sha512":
		h = sha512.New()
	case "sha256":
		h = sha256.New()
	default:
		return "", fmt.Errorf("unsupported algorithm: %s", alg)
	}
	if _, err := io.Copy(h, bytes.NewReader(data)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func parseStoreConn(ctx context.Context, name string) (ocflfs.FS, string, error) {
	rl, err := url.Parse(name)
	if err != nil {
		return nil, "", err
	}
	switch rl.Scheme {
	case "s3":
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, "", err
		}
		fsys := s3.NewBucketFS(awsS3.NewFromConfig(cfg), rl.Host,
			s3.WithLogger(logging.DefaultLogger()))
		return fsys, strings.TrimPrefix(rl.Path, "/"), nil
	default:
		fsys, err := local.NewFS(name)
		if err != nil {
			return nil, "", err
		}
		return fsys, ".", nil
	}
}
