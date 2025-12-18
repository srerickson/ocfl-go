package ocfl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// Activity executor implementations
// These functions implement the actual work for each activity type

func executeDeclareObject(ctx context.Context, objFS ocflfs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
	spec := activity.Params.Spec
	if spec.Empty() {
		return ActivityResult{}, fmt.Errorf("spec not specified in activity params")
	}

	namaste := Namaste{
		Type:    NamasteTypeObject,
		Version: spec,
	}

	// Check if declaration already exists (idempotency)
	declPath := path.Join(objPath, namaste.Name())
	if _, err := ocflfs.StatFile(ctx, objFS, declPath); err == nil {
		return ActivityResult{
			Skipped:    true,
			SkipReason: "declaration file already exists",
		}, nil
	}

	// Write the declaration
	if err := WriteDeclaration(ctx, objFS, objPath, namaste); err != nil {
		return ActivityResult{Error: err}, err
	}

	return ActivityResult{
		BytesWritten: int64(len(namaste.Name())),
	}, nil
}

func executeRemoveDeclaration(ctx context.Context, objFS ocflfs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
	spec := activity.Params.Spec
	if spec.Empty() {
		return ActivityResult{}, fmt.Errorf("spec not specified in activity params")
	}

	namaste := Namaste{
		Type:    NamasteTypeObject,
		Version: spec,
	}

	declPath := path.Join(objPath, namaste.Name())

	// Remove the old declaration (idempotent - doesn't fail if not exists)
	err := ocflfs.Remove(ctx, objFS, declPath)
	if errors.Is(err, fs.ErrNotExist) {
		return ActivityResult{
			Skipped:    true,
			SkipReason: "declaration file does not exist",
		}, nil
	}
	if err != nil {
		return ActivityResult{Error: err}, err
	}

	return ActivityResult{}, nil
}

func executeCreateVersionDir(ctx context.Context, objFS ocflfs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
	versionPath := activity.Params.VersionPath
	if versionPath == "" {
		return ActivityResult{}, fmt.Errorf("version path not specified in activity params")
	}

	fullPath := path.Join(objPath, versionPath)

	// Check if directory already exists (idempotency)
	if stat, err := ocflfs.StatFile(ctx, objFS, fullPath); err == nil {
		if !stat.IsDir() {
			return ActivityResult{Error: fmt.Errorf("version path exists but is not a directory")},
				fmt.Errorf("version path exists but is not a directory")
		}
		return ActivityResult{
			Skipped:    true,
			SkipReason: "version directory already exists",
		}, nil
	}

	// Note: Directory will be created automatically when files are written to it
	// No explicit directory creation needed with the ocflfs abstraction

	return ActivityResult{}, nil
}

func executeCopyContent(ctx context.Context, objFS ocflfs.FS, objPath string, src ContentSource, activity *UpdatePlanActivity) (ActivityResult, error) {
	sourceDigest := activity.Params.SourceDigest
	destPath := activity.Params.DestPath

	if sourceDigest == "" {
		return ActivityResult{}, fmt.Errorf("source digest not specified in activity params")
	}
	if destPath == "" {
		return ActivityResult{}, fmt.Errorf("destination path not specified in activity params")
	}

	// Get content from source
	srcFS, srcPath := src.GetContent(sourceDigest)
	if srcFS == nil {
		return ActivityResult{}, fmt.Errorf("content source doesn't provide digest %q", sourceDigest)
	}

	fullDestPath := path.Join(objPath, destPath)

	// Check if file already exists with correct digest (idempotency)
	if stat, err := ocflfs.StatFile(ctx, objFS, fullDestPath); err == nil {
		// File exists - verify it has the correct content
		if err := verifyFileDigest(ctx, objFS, fullDestPath, sourceDigest); err == nil {
			// File exists and has correct content
			return ActivityResult{
				BytesWritten:   stat.Size(),
				DigestComputed: sourceDigest,
				Skipped:        true,
				SkipReason:     "file already exists with correct content",
			}, nil
		}
		// File exists but has wrong content - this is an error in OCFL context
		// because content-addressed files should never change
		return ActivityResult{}, fmt.Errorf("file exists at %q but has incorrect digest", fullDestPath)
	}

	// Copy the file
	bytesWritten, err := ocflfs.Copy(ctx, objFS, fullDestPath, srcFS, srcPath)
	if err != nil {
		return ActivityResult{Error: err}, err
	}

	// Verify the copied content
	if err := verifyFileDigest(ctx, objFS, fullDestPath, sourceDigest); err != nil {
		// Verification failed - remove the bad file
		_ = ocflfs.Remove(ctx, objFS, fullDestPath)
		return ActivityResult{Error: err}, fmt.Errorf("digest verification failed after copy: %w", err)
	}

	return ActivityResult{
		BytesWritten:   bytesWritten,
		DigestComputed: sourceDigest,
	}, nil
}

func executeWriteVersionInventory(ctx context.Context, objFS ocflfs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
	versionPath := activity.Params.VersionPath
	invJSON := activity.Params.InventoryJSON
	invDigest := activity.Params.InventoryDigest

	if versionPath == "" {
		return ActivityResult{}, fmt.Errorf("version path not specified in activity params")
	}
	if len(invJSON) == 0 {
		return ActivityResult{}, fmt.Errorf("inventory JSON not specified in activity params")
	}

	invPath := path.Join(objPath, versionPath, inventoryBase)

	// Check if inventory already exists with correct content (idempotency)
	if _, err := ocflfs.StatFile(ctx, objFS, invPath); err == nil {
		if err := verifyFileDigest(ctx, objFS, invPath, invDigest); err == nil {
			return ActivityResult{
				BytesWritten: int64(len(invJSON)),
				Skipped:      true,
				SkipReason:   "inventory already exists with correct content",
			}, nil
		}
	}

	// Write the inventory
	bytesWritten, err := ocflfs.Write(ctx, objFS, invPath, bytes.NewReader(invJSON))
	if err != nil {
		return ActivityResult{Error: err}, err
	}

	// Verify written content
	if err := verifyFileDigest(ctx, objFS, invPath, invDigest); err != nil {
		_ = ocflfs.Remove(ctx, objFS, invPath)
		return ActivityResult{Error: err}, fmt.Errorf("digest verification failed after write: %w", err)
	}

	return ActivityResult{
		BytesWritten: bytesWritten,
	}, nil
}

func executeWriteVersionSidecar(ctx context.Context, objFS ocflfs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
	sidecarPath := activity.Params.SidecarPath
	sidecarContent := activity.Params.SidecarContent

	if sidecarPath == "" {
		return ActivityResult{}, fmt.Errorf("sidecar path not specified in activity params")
	}
	if len(sidecarContent) == 0 {
		return ActivityResult{}, fmt.Errorf("sidecar content not specified in activity params")
	}

	fullPath := path.Join(objPath, sidecarPath)

	// Check if sidecar already exists with correct content (idempotency)
	if existing, err := ocflfs.ReadAll(ctx, objFS, fullPath); err == nil {
		if bytes.Equal(existing, sidecarContent) {
			return ActivityResult{
				BytesWritten: int64(len(sidecarContent)),
				Skipped:      true,
				SkipReason:   "sidecar already exists with correct content",
			}, nil
		}
	}

	// Write the sidecar
	bytesWritten, err := ocflfs.Write(ctx, objFS, fullPath, bytes.NewReader(sidecarContent))
	if err != nil {
		return ActivityResult{Error: err}, err
	}

	return ActivityResult{
		BytesWritten: bytesWritten,
	}, nil
}

func executeWriteRootInventory(ctx context.Context, objFS ocflfs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
	invJSON := activity.Params.InventoryJSON
	invDigest := activity.Params.InventoryDigest

	if len(invJSON) == 0 {
		return ActivityResult{}, fmt.Errorf("inventory JSON not specified in activity params")
	}

	invPath := path.Join(objPath, inventoryBase)

	// For root inventory, we always write (not idempotent check) because
	// this represents the final state transition
	bytesWritten, err := ocflfs.Write(ctx, objFS, invPath, bytes.NewReader(invJSON))
	if err != nil {
		return ActivityResult{Error: err}, err
	}

	// Verify written content
	if err := verifyFileDigest(ctx, objFS, invPath, invDigest); err != nil {
		return ActivityResult{Error: err}, fmt.Errorf("digest verification failed after write: %w", err)
	}

	return ActivityResult{
		BytesWritten: bytesWritten,
	}, nil
}

func executeWriteRootSidecar(ctx context.Context, objFS ocflfs.FS, objPath string, activity *UpdatePlanActivity) (ActivityResult, error) {
	sidecarPath := activity.Params.SidecarPath
	sidecarContent := activity.Params.SidecarContent
	algorithm := activity.Params.DigestAlgorithm

	if sidecarPath == "" {
		return ActivityResult{}, fmt.Errorf("sidecar path not specified in activity params")
	}
	if len(sidecarContent) == 0 {
		return ActivityResult{}, fmt.Errorf("sidecar content not specified in activity params")
	}

	fullPath := path.Join(objPath, sidecarPath)

	// Write the sidecar
	bytesWritten, err := ocflfs.Write(ctx, objFS, fullPath, bytes.NewReader(sidecarContent))
	if err != nil {
		return ActivityResult{Error: err}, err
	}

	// If there's an old sidecar with a different algorithm, it should be removed
	// but we don't do that here - that's handled by a separate activity
	_ = algorithm // keep for future use

	return ActivityResult{
		BytesWritten: bytesWritten,
	}, nil
}

// Helper functions

// verifyFileDigest reads a file and verifies it matches the expected digest
func verifyFileDigest(ctx context.Context, fsys ocflfs.FS, filePath, expectedDigest string) error {
	// Read the file content
	content, err := ocflfs.ReadAll(ctx, fsys, filePath)
	if err != nil {
		return err
	}

	// Get the algorithm from the default registry
	// The expected digest should be just the hex value without prefix
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
