package ocfl_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/stage"
)

var (
	objectFixturesPath = filepath.Join(`testdata`, `object-fixtures`)
	storeFixturePath   = filepath.Join(`testdata`, `store-fixtures`)
)

// testStage is the kind of stage type implementers can build from the
// package's pieces: an update whose new content comes from files and memory,
// with methods to add both.
type testStage struct {
	*ocfl.ObjectUpdate
	files stage.FileContentMap
	mem   stage.BytesContentMap
}

// newTestStage returns a stage for a new update to the object at dir in fsys.
func newTestStage(ctx context.Context, fsys ocflfs.FS, dir, id string, opts ...ocfl.NewUpdateOption) (*testStage, error) {
	u, _, err := ocfl.NewObjectUpdate(ctx, fsys, dir, id, opts...)
	if err != nil {
		return nil, err
	}
	return stageFor(u), nil
}

// newRootStage returns a stage for a new update to the object id in root.
func newRootStage(ctx context.Context, root *ocfl.Root, id string, opts ...ocfl.NewUpdateOption) (*testStage, error) {
	u, _, err := root.NewObjectUpdate(ctx, id, opts...)
	if err != nil {
		return nil, err
	}
	return stageFor(u), nil
}

// stageFor returns a stage for u with no content.
func stageFor(u *ocfl.ObjectUpdate) *testStage { return &testStage{ObjectUpdate: u} }

func (s *testStage) Update() *ocfl.ObjectUpdate { return s.ObjectUpdate }

func (s *testStage) Content() ocfl.ContentSource {
	return multiSource{&s.files, &s.mem}
}

func (s *testStage) AddFS(ctx context.Context, fsys ocflfs.FS, dir, dst string, opts ...stage.Option) error {
	return stage.FS(ctx, s.ObjectUpdate, &s.files, fsys, dir, dst, opts...)
}

func (s *testStage) AddFile(ctx context.Context, fsys ocflfs.FS, name, dst string, opts ...stage.Option) error {
	return stage.File(ctx, s.ObjectUpdate, &s.files, fsys, name, dst, opts...)
}

func (s *testStage) AddBytes(dst string, b []byte, fixity ...digest.Algorithm) error {
	return stage.Bytes(s.ObjectUpdate, &s.mem, dst, b, fixity...)
}

// multiSource gets content from the first of its sources that has it.
type multiSource []ocfl.ContentSource

func (m multiSource) GetContent(dig string) (ocflfs.FS, string) {
	for _, src := range m {
		if fsys, name := src.GetContent(dig); fsys != nil {
			return fsys, name
		}
	}
	return nil, ""
}

// stageBytes clears stage and adds the files in content to it.
func stageBytes(t *testing.T, stage *testStage, content map[string][]byte, fixity ...digest.Algorithm) *testStage {
	t.Helper()
	be.NilErr(t, stage.Clear())
	for name, b := range content {
		be.NilErr(t, stage.AddBytes(name, b, fixity...))
	}
	return stage
}

// commit finalizes the stage's update and applies it to obj's storage,
// returning the updated object.
func commit(t *testing.T, obj *ocfl.Object, stage *testStage, msg string, opts ...ocfl.FinalizeUpdateOption) *ocfl.Object {
	t.Helper()
	ctx := context.Background()
	be.NilErr(t, stage.Finalize(msg, ocfl.User{Name: "Tester"}, opts...))
	newObj, err := stage.Update().Apply(ctx, obj.FS(), obj.Path(), stage.Content())
	be.NilErr(t, err)
	return newObj
}
