package ocfl_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/srerickson/ocfl-go"
)

func TestStageDir(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.DirFS("testdata")
	stage, err := ocfl.StageDir(ctx, fsys, "content-fixture", ocfl.SHA256, ocfl.MD5)
	if err != nil {
		t.Fatal(err)
	}
	if stage.DigestAlgorithm != ocfl.SHA256 {
		t.Fatalf("stage's alg isn't %s", ocfl.SHA256)
	}
	expectedSize := 3
	if got := len(stage.State); got != expectedSize {
		t.Fatalf("expected %d, got %d", expectedSize, got)
	}
	expectedPath := "folder1/folder2/sculpture-stone-face-head-888027.jpg"
	digest := stage.State.GetDigest(expectedPath)
	if digest == "" {
		t.Logf("staged paths: %s", strings.Join(stage.State.Paths(), ", "))
		t.Fatalf("stage state doesn't include digest for %q as expected", expectedPath)
	}
	resolveFS, resolvePath := stage.GetContent(digest)
	if resolveFS == nil {
		t.Fatal("the stage's content resolver returned nil FS")
	}
	resolveFile, err := resolveFS.OpenFile(ctx, resolvePath)
	if err != nil {
		t.Fatalf("opening staged file %q: %v", resolvePath, err)
	}
	defer resolveFile.Close()
	if _, err := io.Copy(io.Discard, resolveFile); err != nil {
		t.Fatalf("reading staged file: %v", err)
	}
	fixity := stage.FixitySource.GetFixity(digest)
	if _, ok := fixity[ocfl.MD5]; !ok {
		t.Fatalf("fixity doesn't have md5 value")
	}

}
