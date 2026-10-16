package stage_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/stage"
)

var (
	testdataPath       = filepath.Join(`..`, `testdata`)
	objectFixturesPath = filepath.Join(testdataPath, `object-fixtures`)
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
	return &testStage{ObjectUpdate: u}, nil
}

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

// mustMarshal returns v encoded as JSON.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	be.NilErr(t, err)
	return b
}
