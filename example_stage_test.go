package ocfl_test

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
)

// uploadDir is a ContentSource for content that has already been uploaded to
// a directory, in files named by their sha512 digests.
type uploadDir struct {
	fsys ocflfs.FS
	dir  string
}

func (u uploadDir) GetContent(dig string) (ocflfs.FS, string) {
	return u.fsys, path.Join(u.dir, dig)
}

// uploadStage is a stage type of our own: files are added by name and digest,
// as they are listed in an upload manifest, without reading them again.
type uploadStage struct {
	ocfl.Stage[uploadDir]
}

func (s *uploadStage) AddUploaded(name, sha512 string) error {
	return s.Update.Add(name, sha512, nil)
}

func Example_customStage() {
	ctx := context.Background()
	tmp, err := os.MkdirTemp("", "ocfl-example-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	fsys, err := local.NewFS(tmp)
	if err != nil {
		log.Fatal(err)
	}
	// a client uploaded a file and sent its digest
	full := sha512.Sum512([]byte("hello"))
	dig := hex.EncodeToString(full[:])
	if err := os.MkdirAll(filepath.Join(tmp, "uploads"), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "uploads", dig), []byte("hello"), 0o644); err != nil {
		log.Fatal(err)
	}

	update, err := ocfl.NewUpdate(ctx, fsys, "object", "ark:/example")
	if err != nil {
		log.Fatal(err)
	}
	stage := uploadStage{ocfl.Stage[uploadDir]{
		Update:  update,
		Content: uploadDir{fsys: fsys, dir: "uploads"},
	}}
	if err := stage.AddUploaded("greeting.txt", dig); err != nil {
		log.Fatal(err)
	}
	if err := stage.Update.Finalize("first version", ocfl.User{Name: "Example"}); err != nil {
		log.Fatal(err)
	}
	obj, err := stage.Apply(ctx, fsys, "object")
	if err != nil {
		log.Fatal(err)
	}
	vfs, err := obj.VersionFS(ctx, 0)
	if err != nil {
		log.Fatal(err)
	}
	b, err := fs.ReadFile(vfs, "greeting.txt")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(obj.ID(), obj.Head(), string(b))
	// Output: ark:/example v1 hello
}
