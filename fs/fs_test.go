package fs_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/srerickson/ocfl-go/fs"
	httpfs "github.com/srerickson/ocfl-go/fs/http"
	"github.com/srerickson/ocfl-go/fs/local"
)

func TestFS_Eq(t *testing.T) {
	t.Run("local.FS equality", func(t *testing.T) {
		// Create two FS with the same path
		fs1, err := local.NewFS("/tmp")
		if err != nil {
			t.Fatal(err)
		}
		fs2, err := local.NewFS("/tmp")
		if err != nil {
			t.Fatal(err)
		}
		
		// They should be equal
		if !fs1.Eq(fs2) {
			t.Error("Expected fs1.Eq(fs2) to be true for same paths")
		}
		if !fs2.Eq(fs1) {
			t.Error("Expected fs2.Eq(fs1) to be true for same paths")
		}
		
		// Create FS with different path
		fs3, err := local.NewFS("/")
		if err != nil {
			t.Fatal(err)
		}
		
		// They should not be equal
		if fs1.Eq(fs3) {
			t.Error("Expected fs1.Eq(fs3) to be false for different paths")
		}
		if fs3.Eq(fs1) {
			t.Error("Expected fs3.Eq(fs1) to be false for different paths")
		}
	})

	t.Run("http.FS equality", func(t *testing.T) {
		// Create two FS with the same baseURL and client
		fs1 := httpfs.New("https://example.com", httpfs.WithClient(http.DefaultClient))
		fs2 := httpfs.New("https://example.com", httpfs.WithClient(http.DefaultClient))
		
		// They should be equal
		if !fs1.Eq(fs2) {
			t.Error("Expected fs1.Eq(fs2) to be true for same baseURL and client")
		}
		
		// Create FS with different baseURL
		fs3 := httpfs.New("https://other.com", httpfs.WithClient(http.DefaultClient))
		
		// They should not be equal
		if fs1.Eq(fs3) {
			t.Error("Expected fs1.Eq(fs3) to be false for different baseURL")
		}
		
		// Create FS with different client
		customClient := &http.Client{}
		fs4 := httpfs.New("https://example.com", httpfs.WithClient(customClient))
		
		// They should not be equal
		if fs1.Eq(fs4) {
			t.Error("Expected fs1.Eq(fs4) to be false for different client")
		}
	})

	t.Run("WrapFS equality", func(t *testing.T) {
		// Create two WrapFS with the same underlying fs.FS
		osFS := os.DirFS("/tmp")
		fs1 := fs.NewWrapFS(osFS)
		fs2 := fs.NewWrapFS(osFS)
		
		// They should be equal
		if !fs1.Eq(fs2) {
			t.Error("Expected fs1.Eq(fs2) to be true for same underlying fs.FS")
		}
		
		// Create WrapFS with different underlying fs.FS
		otherFS := os.DirFS("/")
		fs3 := fs.NewWrapFS(otherFS)
		
		// They should not be equal
		if fs1.Eq(fs3) {
			t.Error("Expected fs1.Eq(fs3) to be false for different underlying fs.FS")
		}
	})

	t.Run("Different FS types", func(t *testing.T) {
		// Create different types of FS
		localFS, err := local.NewFS("/tmp")
		if err != nil {
			t.Fatal(err)
		}
		httpFS := httpfs.New("https://example.com")
		wrapFS := fs.NewWrapFS(os.DirFS("/tmp"))
		
		// They should not be equal
		if localFS.Eq(httpFS) {
			t.Error("Expected localFS.Eq(httpFS) to be false for different types")
		}
		if httpFS.Eq(wrapFS) {
			t.Error("Expected httpFS.Eq(wrapFS) to be false for different types")
		}
		if wrapFS.Eq(localFS) {
			t.Error("Expected wrapFS.Eq(localFS) to be false for different types")
		}
	})

	t.Run("Nil comparison", func(t *testing.T) {
		localFS, err := local.NewFS("/tmp")
		if err != nil {
			t.Fatal(err)
		}
		
		// Should handle nil comparison
		if localFS.Eq(nil) {
			t.Error("Expected localFS.Eq(nil) to be false")
		}
	})
}

func TestFS_Copy_UsesEq(t *testing.T) {
	ctx := context.Background()
	
	t.Run("Copy between different FS uses manual path", func(t *testing.T) {
		// Create two different local.FS instances
		tmpDir1 := t.TempDir()
		tmpDir2 := t.TempDir()
		
		srcFS, err := local.NewFS(tmpDir1)
		if err != nil {
			t.Fatal(err)
		}
		dstFS, err := local.NewFS(tmpDir2)
		if err != nil {
			t.Fatal(err)
		}
		
		// Verify they are not equal using the new Eq method
		if srcFS.Eq(dstFS) {
			t.Fatal("Expected srcFS.Eq(dstFS) to be false for different paths")
		}
		
		// Create a test file
		content := "test content"
		_, err = srcFS.Write(ctx, "test.txt", strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		
		// Copying between different FS should use manual copy
		// Since local.FS doesn't implement CopyFS, this will always use
		// the manual copy path, but the important thing is that Eq() 
		// is being called to determine this
		size, err := fs.Copy(ctx, dstFS, "copy.txt", srcFS, "test.txt")
		if err != nil {
			t.Fatal(err)
		}
		
		if size != int64(len(content)) {
			t.Errorf("Expected size %d, got %d", len(content), size)
		}
		
		// Verify the file was copied to the different directory
		copiedContent, err := fs.ReadAll(ctx, dstFS, "copy.txt")
		if err != nil {
			t.Fatal(err)
		}
		
		if string(copiedContent) != content {
			t.Errorf("Expected content %q, got %q", content, string(copiedContent))
		}
	})
	
	t.Run("Copy within same FS", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir := t.TempDir()
		
		// Create two local.FS instances pointing to the same directory
		srcFS, err := local.NewFS(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		dstFS, err := local.NewFS(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		
		// Verify they are equal according to our new Eq method
		if !srcFS.Eq(dstFS) {
			t.Fatal("Expected srcFS.Eq(dstFS) to be true for same paths")
		}
		
		// Create a test file
		content := "test content for same FS"
		_, err = srcFS.Write(ctx, "test.txt", strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		
		// Copy within the same FS should work (even though local.FS 
		// doesn't implement CopyFS, the Eq check is still performed)
		size, err := fs.Copy(ctx, dstFS, "copy.txt", srcFS, "test.txt")
		if err != nil {
			t.Fatal(err)
		}
		
		if size != int64(len(content)) {
			t.Errorf("Expected size %d, got %d", len(content), size)
		}
		
		// Verify the file was copied
		copiedContent, err := fs.ReadAll(ctx, dstFS, "copy.txt")
		if err != nil {
			t.Fatal(err)
		}
		
		if string(copiedContent) != content {
			t.Errorf("Expected content %q, got %q", content, string(copiedContent))
		}
	})
}