package ocfl_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestNewUpdate(t *testing.T) {
	ctx := context.Background()
	goodObjects := filepath.Join(objectFixturesPath, `1.1`, `good-objects`)
	badObjects := filepath.Join(objectFixturesPath, `1.1`, `bad-objects`)
	t.Run("new object", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj-1")
		be.NilErr(t, err)
		be.Equal(t, "obj-1", upd.ID())
		be.Equal(t, digest.SHA512.ID(), upd.DigestAlgorithm().ID())
		be.Equal(t, ocfl.V(1), upd.NextHead())
		be.Zero(t, upd.BaseInventoryDigest())
		be.Equal(t, 0, len(upd.State()))
		be.Equal(t, 0, len(upd.BaseState()))
		be.False(t, upd.Finalized())
	})
	t.Run("new object requires an ID", func(t *testing.T) {
		_, err := ocfl.NewUpdate(ctx, testutil.TmpLocalFS(t), "obj", "")
		be.True(t, errors.Is(err, ocfl.ErrNoObjectID))
	})
	t.Run("new object with sha256", func(t *testing.T) {
		upd, err := ocfl.NewUpdate(ctx, testutil.TmpLocalFS(t), "obj", "obj-1",
			ocfl.UpdateWithDigestAlgorithm(digest.SHA256))
		be.NilErr(t, err)
		be.Equal(t, digest.SHA256.ID(), upd.DigestAlgorithm().ID())
	})
	t.Run("unsupported digest algorithm", func(t *testing.T) {
		_, err := ocfl.NewUpdate(ctx, testutil.TmpLocalFS(t), "obj", "obj-1",
			ocfl.UpdateWithDigestAlgorithm(digest.MD5))
		be.Nonzero(t, err)
		be.In(t, "md5", err.Error())
	})
	t.Run("existing object", func(t *testing.T) {
		fsys := ocflfs.DirFS(goodObjects)
		obj, err := ocfl.NewObject(ctx, fsys, "spec-ex-full")
		be.NilErr(t, err)
		for _, id := range []string{"", obj.ID()} {
			upd, err := ocfl.NewUpdate(ctx, fsys, "spec-ex-full", id)
			be.NilErr(t, err)
			be.Equal(t, obj.ID(), upd.ID())
			be.Equal(t, obj.InventoryDigest(), upd.BaseInventoryDigest())
			be.Equal(t, ocfl.V(4), upd.NextHead())
			be.True(t, obj.Version(0).State().Eq(upd.State()))
			be.True(t, obj.Version(0).State().Eq(upd.BaseState()))
		}
	})
	t.Run("base state is a copy", func(t *testing.T) {
		upd, err := ocfl.NewUpdate(ctx, ocflfs.DirFS(goodObjects), "spec-ex-full", "")
		be.NilErr(t, err)
		base := upd.BaseState()
		be.NilErr(t, upd.Clear())
		be.Equal(t, 0, len(upd.State()))
		be.True(t, base.Eq(upd.BaseState()))
		clear(base)
		be.Nonzero(t, len(upd.BaseState()))
	})
	t.Run("existing object ignores digest algorithm", func(t *testing.T) {
		// spec-ex-full uses sha512
		upd, err := ocfl.NewUpdate(ctx, ocflfs.DirFS(goodObjects), "spec-ex-full", "",
			ocfl.UpdateWithDigestAlgorithm(digest.SHA256))
		be.NilErr(t, err)
		be.Equal(t, digest.SHA512.ID(), upd.DigestAlgorithm().ID())
	})
	t.Run("existing object with wrong ID", func(t *testing.T) {
		_, err := ocfl.NewUpdate(ctx, ocflfs.DirFS(goodObjects), "spec-ex-full", "other-id")
		be.Nonzero(t, err)
		be.In(t, "unexpected ID", err.Error())
	})
	t.Run("not an object", func(t *testing.T) {
		_, err := ocfl.NewUpdate(ctx, ocflfs.DirFS(`testdata`), "content-fixture", "obj-1")
		be.Nonzero(t, err)
		be.In(t, "non-conforming", err.Error())
	})
	t.Run("missing inventory", func(t *testing.T) {
		_, err := ocfl.NewUpdate(ctx, ocflfs.DirFS(badObjects), "E063_no_inv", "obj-1")
		be.True(t, errors.Is(err, ocfl.ErrObjectIncomplete))
	})
	t.Run("root inventory doesn't match sidecar", func(t *testing.T) {
		_, err := ocfl.NewUpdate(ctx, ocflfs.DirFS(badObjects), "E060_E064_root_inventory_digest_mismatch", "")
		be.True(t, errors.Is(err, ocfl.ErrObjectIncomplete))
		var digestErr *digest.DigestError
		be.True(t, errors.As(err, &digestErr))
	})
}

func TestObjectUpdate_Edits(t *testing.T) {
	newUpdate := func(t *testing.T) *ocfl.ObjectUpdate {
		upd, err := ocfl.NewUpdate(context.Background(), testutil.TmpLocalFS(t), "obj", "obj")
		be.NilErr(t, err)
		return upd
	}
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	t.Run("add", func(t *testing.T) {
		upd := newUpdate(t)
		be.NilErr(t, upd.Add("a.txt", strings.ToUpper(digA), digest.Set{"md5": "ABC", "sha512": digA}))
		be.NilErr(t, upd.Add("dir/a.txt", digA, digest.Set{"size": "1"}))
		be.DeepEqual(t, ocfl.DigestMap{digA: {"a.txt", "dir/a.txt"}}, upd.State())
		// fixity is normalized and doesn't include the primary algorithm
		be.DeepEqual(t, digest.Set{"md5": "abc", "size": "1"}, upd.Fixity(digA))
		// replace a.txt
		be.NilErr(t, upd.Add("a.txt", digB, nil))
		be.DeepEqual(t, ocfl.DigestMap{digA: {"dir/a.txt"}, digB: {"a.txt"}}, upd.State())
		// fixity with only the primary algorithm isn't saved
		be.NilErr(t, upd.Add("c.txt", strings.Repeat("c", 128), digest.Set{"sha512": strings.Repeat("c", 128)}))
		be.Zero(t, upd.Fixity(strings.Repeat("c", 128)))
		be.NilErr(t, upd.Remove("c.txt"))
		// fixity is dropped when the content is removed
		be.NilErr(t, upd.Add("dir/a.txt", digB, nil))
		be.Zero(t, upd.Fixity(digA))
	})
	t.Run("add errors", func(t *testing.T) {
		upd := newUpdate(t)
		be.NilErr(t, upd.Add("dir/a.txt", digA, digest.Set{"md5": "abc"}))
		for name, args := range map[string][2]string{
			"invalid path":     {"../a.txt", digA},
			"dot path":         {".", digA},
			"file is dir":      {"dir", digB},
			"dir is file":      {"dir/a.txt/b.txt", digB},
			"wrong length":     {"b.txt", strings.Repeat("a", 64)},
			"not hex":          {"b.txt", strings.Repeat("g", 128)},
			"empty digest":     {"b.txt", ""},
			"conflicting path": {"dir/a.txt/", digB},
		} {
			t.Run(name, func(t *testing.T) {
				be.Nonzero(t, upd.Add(args[0], args[1], nil))
			})
		}
		t.Run("conflicting fixity", func(t *testing.T) {
			be.Nonzero(t, upd.Add("b.txt", digA, digest.Set{"md5": "def"}))
		})
		t.Run("fixity with a different primary digest", func(t *testing.T) {
			err := upd.Add("b.txt", digA, digest.Set{"sha512": digB})
			be.Nonzero(t, err)
			be.In(t, "different sha512 digest", err.Error())
		})
		// state is unchanged
		be.DeepEqual(t, ocfl.DigestMap{digA: {"dir/a.txt"}}, upd.State())
	})
	t.Run("remove", func(t *testing.T) {
		upd := newUpdate(t)
		be.NilErr(t, upd.Add("a.txt", digA, nil))
		be.NilErr(t, upd.Add("dir/b.txt", digB, nil))
		be.NilErr(t, upd.Add("dir/sub/a.txt", digA, nil))
		be.NilErr(t, upd.Add("dir2/c.txt", digB, nil))
		be.NilErr(t, upd.Remove("a.txt"))
		be.DeepEqual(t, []string{"dir/b.txt", "dir/sub/a.txt", "dir2/c.txt"}, upd.State().AllPaths())
		be.NilErr(t, upd.Remove("dir"))
		be.DeepEqual(t, []string{"dir2/c.txt"}, upd.State().AllPaths())
		be.True(t, errors.Is(upd.Remove("dir"), fs.ErrNotExist))
		be.True(t, errors.Is(upd.Remove("dir2/c"), fs.ErrNotExist))
		be.Nonzero(t, upd.Remove("."))
	})
	t.Run("rename", func(t *testing.T) {
		upd := newUpdate(t)
		be.NilErr(t, upd.Add("a.txt", digA, nil))
		be.NilErr(t, upd.Add("dir/b.txt", digB, nil))
		be.NilErr(t, upd.Rename("a.txt", "c.txt"))
		be.DeepEqual(t, []string{"c.txt", "dir/b.txt"}, upd.State().AllPaths())
		be.NilErr(t, upd.Rename("dir", "new/dir"))
		be.DeepEqual(t, []string{"c.txt", "new/dir/b.txt"}, upd.State().AllPaths())
		be.NilErr(t, upd.Rename(".", "top"))
		be.DeepEqual(t, []string{"top/c.txt", "top/new/dir/b.txt"}, upd.State().AllPaths())
		be.True(t, errors.Is(upd.Rename("missing", "x"), fs.ErrNotExist))
		// conflict: a file can't be a directory
		be.Nonzero(t, upd.Rename("top/c.txt", "top/new"))
		be.DeepEqual(t, []string{"top/c.txt", "top/new/dir/b.txt"}, upd.State().AllPaths())
	})
	t.Run("clear", func(t *testing.T) {
		upd := newUpdate(t)
		be.NilErr(t, upd.Add("a.txt", digA, digest.Set{"md5": "abc"}))
		be.NilErr(t, upd.Clear())
		be.Equal(t, 0, len(upd.State()))
		be.Zero(t, upd.Fixity(digA))
	})
	t.Run("finalized", func(t *testing.T) {
		upd := newUpdate(t)
		be.NilErr(t, upd.Add("a.txt", digA, nil))
		be.NilErr(t, upd.Finalize("msg", ocfl.User{Name: "Tester"}))
		be.True(t, upd.Finalized())
		be.True(t, errors.Is(upd.Add("b.txt", digB, nil), ocfl.ErrFinalized))
		be.True(t, errors.Is(upd.Remove("a.txt"), ocfl.ErrFinalized))
		be.True(t, errors.Is(upd.Rename("a.txt", "b.txt"), ocfl.ErrFinalized))
		be.True(t, errors.Is(upd.Clear(), ocfl.ErrFinalized))
		be.True(t, errors.Is(upd.Finalize("msg", ocfl.User{Name: "Tester"}), ocfl.ErrFinalized))
		be.DeepEqual(t, []string{"a.txt"}, upd.State().AllPaths())
	})
}

func TestObjectUpdate_Finalize(t *testing.T) {
	ctx := context.Background()
	user := ocfl.User{Name: "Tester", Address: "mailto:tester@example.org"}
	t.Run("new object", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		stage := ocfl.NewStage(upd)
		be.NilErr(t, stage.AddBytes("a.txt", []byte("a"), digest.MD5))
		be.NilErr(t, stage.AddBytes("b.txt", []byte("a")))
		created := time.Date(2026, 10, 4, 12, 0, 0, 999, time.FixedZone("PDT", -7*60*60))
		_, ok := upd.VersionInfo()
		be.False(t, ok)
		be.NilErr(t, upd.Finalize("first version", user, ocfl.UpdateWithVersionCreated(created)))
		be.Nonzero(t, upd.NewInventoryDigest())
		info, ok := upd.VersionInfo()
		be.True(t, ok)
		be.Equal(t, "first version", info.Message)
		be.Equal(t, user, info.User)
		be.Equal(t, ocfl.Spec1_1, info.Spec)
		// created is truncated to the second and keeps its UTC offset
		be.True(t, created.Truncate(time.Second).Equal(info.Created))
		be.Equal(t, 0, info.Created.Nanosecond())
		_, offset := info.Created.Zone()
		be.Equal(t, -7*60*60, offset)
		obj, err := upd.Apply(ctx, fsys, "obj", stage.Content)
		be.NilErr(t, err)
		be.Equal(t, upd.NewInventoryDigest(), obj.InventoryDigest())
		ver := obj.Version(1)
		be.Equal(t, "first version", ver.Message())
		be.Equal(t, user, *ver.User())
		be.True(t, created.Truncate(time.Second).Equal(ver.Created()))
		be.Equal(t, ocfl.Spec1_1, obj.Spec())
		dig := ver.State().DigestFor("a.txt")
		// content paths for new content are based on its logical paths
		be.DeepEqual(t, ocfl.DigestMap{dig: {"v1/content/a.txt", "v1/content/b.txt"}}, obj.Manifest())
		be.DeepEqual(t, []string{"md5"}, obj.FixityAlgorithms())
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, "obj").Err())
	})
	t.Run("content path func", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		stage := ocfl.NewStage(upd)
		be.NilErr(t, stage.AddBytes("dir/a.txt", []byte("a")))
		be.NilErr(t, upd.Finalize("msg", user, ocfl.UpdateWithContentPathFunc(func(paths []string) []string {
			for i, p := range paths {
				paths[i] = strings.ToUpper(p)
			}
			return paths
		})))
		obj, err := upd.Apply(ctx, fsys, "obj", stage.Content)
		be.NilErr(t, err)
		be.DeepEqual(t, []string{"v1/content/DIR/A.TXT"}, obj.Manifest().AllPaths())
		// the content path func isn't needed after finalize
		saved, err := json.Marshal(upd)
		be.NilErr(t, err)
		var loaded ocfl.ObjectUpdate
		be.NilErr(t, json.Unmarshal(saved, &loaded))
		be.Equal(t, upd.NewInventoryDigest(), loaded.NewInventoryDigest())
	})
	t.Run("existing object keeps padding, content directory, and fixity", func(t *testing.T) {
		fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `minimal_content_dir_called_stuff`)
		fsys := testutil.TmpLocalFS(t, fixture)
		obj, err := ocfl.NewObject(ctx, fsys, "minimal_content_dir_called_stuff")
		be.NilErr(t, err)
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new.txt", []byte("new"), digest.MD5))
		newObj := commit(t, obj, stage, "v2")
		be.Equal(t, "stuff", newObj.ContentDirectory())
		be.Equal(t, obj.Head().Padding(), newObj.Head().Padding())
		be.Equal(t, 2, newObj.Head().Num())
		dig := newObj.Version(0).State().DigestFor("new.txt")
		be.DeepEqual(t, []string{"v2/stuff/new.txt"}, newObj.Manifest()[dig])
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, newObj.Path()).Err())
	})
	t.Run("new head", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		err = upd.Finalize("msg", user, ocfl.UpdateWithNewHead(2))
		be.True(t, errors.Is(err, ocfl.ErrUnexpectedHead))
		be.In(t, "expected update to create version 2", err.Error())
		be.In(t, "would create version 1", err.Error())
		be.False(t, upd.Finalized())
		be.NilErr(t, upd.Finalize("msg", user, ocfl.UpdateWithNewHead(1)))
	})
	t.Run("unchanged state", func(t *testing.T) {
		fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `spec-ex-full`)
		obj, err := ocfl.NewObject(ctx, ocflfs.DirFS(fixture), ".")
		be.NilErr(t, err)
		upd := obj.NewUpdate()
		err = upd.Finalize("msg", user)
		be.Nonzero(t, err)
		be.In(t, "unchanged version state", err.Error())
		be.NilErr(t, upd.Finalize("msg", user, ocfl.UpdateWithUnchangedVersionState()))
	})
	t.Run("spec can't be lowered", func(t *testing.T) {
		fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `spec-ex-full`)
		obj, err := ocfl.NewObject(ctx, ocflfs.DirFS(fixture), ".")
		be.NilErr(t, err)
		upd := obj.NewUpdate()
		err = upd.Finalize("msg", user, ocfl.UpdateWithOCFLSpec(ocfl.Spec1_0), ocfl.UpdateWithUnchangedVersionState())
		be.Nonzero(t, err)
		be.False(t, upd.Finalized())
	})
	t.Run("unknown spec", func(t *testing.T) {
		upd, err := ocfl.NewUpdate(ctx, testutil.TmpLocalFS(t), "obj", "obj")
		be.NilErr(t, err)
		be.Nonzero(t, upd.Finalize("msg", user, ocfl.UpdateWithOCFLSpec(ocfl.Spec("2.0"))))
	})
}

func TestObjectUpdate_JSON(t *testing.T) {
	ctx := context.Background()
	user := ocfl.User{Name: "Tester", Address: "mailto:tester@example.org"}
	fixture := filepath.Join(objectFixturesPath, `1.0`, `good-objects`, `spec-ex-full`)
	// newStage returns a stage for an update to the fixture
	newStage := func(t *testing.T) (*local.FS, *ocfl.Stage) {
		fsys := testutil.TmpLocalFS(t, fixture)
		upd, err := ocfl.NewUpdate(ctx, fsys, "spec-ex-full", "")
		be.NilErr(t, err)
		stage := ocfl.NewStage(upd)
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a"), digest.MD5, digest.SIZE))
		be.NilErr(t, stage.AddBytes("new/b.txt", []byte("b"), digest.MD5))
		be.NilErr(t, stage.Rename("foo", "bar"))
		return fsys, stage
	}
	roundTrip := func(t *testing.T, upd *ocfl.ObjectUpdate) *ocfl.ObjectUpdate {
		t.Helper()
		saved, err := json.Marshal(upd)
		be.NilErr(t, err)
		var loaded ocfl.ObjectUpdate
		be.NilErr(t, json.Unmarshal(saved, &loaded))
		resaved, err := json.Marshal(&loaded)
		be.NilErr(t, err)
		be.Equal(t, string(saved), string(resaved))
		return &loaded
	}
	t.Run("draft", func(t *testing.T) {
		_, stage := newStage(t)
		loaded := roundTrip(t, stage.Update)
		be.False(t, loaded.Finalized())
		_, ok := loaded.VersionInfo()
		be.False(t, ok)
		be.Equal(t, stage.Update.ID(), loaded.ID())
		be.Equal(t, stage.Update.BaseInventoryDigest(), loaded.BaseInventoryDigest())
		be.True(t, stage.Update.State().Eq(loaded.State()))
		// the base state is the fixture's head state, without the draft's edits
		baseState := loaded.BaseState()
		be.True(t, stage.Update.BaseState().Eq(baseState))
		be.DeepEqual(t, []string{"empty2.txt", "foo/bar.xml", "image.tiff"}, baseState.AllPaths())
		be.False(t, baseState.Eq(loaded.State()))
		dig := loaded.State().DigestFor("new/a.txt")
		be.DeepEqual(t, stage.Update.Fixity(dig), loaded.Fixity(dig))
		// a loaded draft can be edited
		be.NilErr(t, loaded.Remove("new/a.txt"))
	})
	t.Run("draft for new object", func(t *testing.T) {
		upd, err := ocfl.NewUpdate(ctx, testutil.TmpLocalFS(t), "obj", "obj",
			ocfl.UpdateWithDigestAlgorithm(digest.SHA256))
		be.NilErr(t, err)
		loaded := roundTrip(t, upd)
		be.Equal(t, digest.SHA256.ID(), loaded.DigestAlgorithm().ID())
		be.Zero(t, loaded.BaseInventoryDigest())
		be.Equal(t, 0, len(loaded.BaseState()))
	})
	t.Run("finalized", func(t *testing.T) {
		fsys, stage := newStage(t)
		created := time.Date(2026, 10, 4, 12, 0, 0, 0, time.FixedZone("", 5*60*60+30*60))
		be.NilErr(t, stage.Update.Finalize("v4", user,
			ocfl.UpdateWithVersionCreated(created), ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1)))
		loaded := roundTrip(t, stage.Update)
		be.True(t, loaded.Finalized())
		be.Equal(t, stage.Update.NewInventoryDigest(), loaded.NewInventoryDigest())
		be.True(t, stage.Update.BaseState().Eq(loaded.BaseState()))
		be.DeepEqual(t, []string{"empty2.txt", "foo/bar.xml", "image.tiff"}, loaded.BaseState().AllPaths())
		info, ok := loaded.VersionInfo()
		be.True(t, ok)
		be.Equal(t, "v4", info.Message)
		be.Equal(t, user, info.User)
		be.Equal(t, ocfl.Spec1_1, info.Spec)
		be.True(t, created.Equal(info.Created))
		_, offset := info.Created.Zone()
		be.Equal(t, 5*60*60+30*60, offset)
		// the loaded update is applied
		obj, err := loaded.Apply(ctx, fsys, "spec-ex-full", stage.Content)
		be.NilErr(t, err)
		be.Equal(t, stage.Update.NewInventoryDigest(), obj.InventoryDigest())
		be.True(t, created.Equal(obj.Version(0).Created()))
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, "spec-ex-full").Err())
	})
	t.Run("saved form", func(t *testing.T) {
		_, stage := newStage(t)
		be.NilErr(t, stage.Update.Finalize("v4", user))
		saved, err := json.Marshal(stage.Update)
		be.NilErr(t, err)
		var fields map[string]json.RawMessage
		be.NilErr(t, json.Unmarshal(saved, &fields))
		be.DeepEqual(t, []string{"base_inventory", "digest_algorithm", "finalized", "fixity", "id", "state"}, sortedKeys(fields))
		// the base inventory is saved as a string with the exact bytes of the
		// object's inventory.json
		var baseInv string
		be.NilErr(t, json.Unmarshal(fields["base_inventory"], &baseInv))
		invBytes, err := os.ReadFile(filepath.Join(fixture, "inventory.json"))
		be.NilErr(t, err)
		be.Equal(t, string(invBytes), baseInv)
		var final map[string]json.RawMessage
		be.NilErr(t, json.Unmarshal(fields["finalized"], &final))
		be.DeepEqual(t, []string{"content_paths", "created", "message", "new_inventory_digest", "spec", "user"}, sortedKeys(final))
		var contentPaths ocfl.DigestMap
		be.NilErr(t, json.Unmarshal(final["content_paths"], &contentPaths))
		be.DeepEqual(t, []string{"v4/content/new/a.txt", "v4/content/new/b.txt"}, contentPaths.AllPaths())
	})
	t.Run("digest algorithm doesn't match base inventory", func(t *testing.T) {
		_, stage := newStage(t)
		saved, err := json.Marshal(stage.Update)
		be.NilErr(t, err)
		var fields map[string]any
		be.NilErr(t, json.Unmarshal(saved, &fields))
		fields["digest_algorithm"] = "sha256"
		changed, err := json.Marshal(fields)
		be.NilErr(t, err)
		var loaded ocfl.ObjectUpdate
		err = json.Unmarshal(changed, &loaded)
		be.Nonzero(t, err)
		be.In(t, "isn't supported", err.Error())
	})
	t.Run("tampering", func(t *testing.T) {
		_, stage := newStage(t)
		be.NilErr(t, stage.Update.Finalize("v4", user))
		saved, err := json.Marshal(stage.Update)
		be.NilErr(t, err)
		digA := stage.Update.State().DigestFor("new/a.txt")
		edits := map[string]func(u map[string]any){
			"state": func(u map[string]any) {
				state := u["state"].(map[string]any)
				state[digA] = []any{"new/c.txt"}
			},
			"fixity": func(u map[string]any) {
				u["fixity"].(map[string]any)[digA].(map[string]any)["md5"] = "00000000000000000000000000000000"
			},
			"message": func(u map[string]any) {
				u["finalized"].(map[string]any)["message"] = "changed"
			},
			"user": func(u map[string]any) {
				u["finalized"].(map[string]any)["user"].(map[string]any)["name"] = "changed"
			},
			"created": func(u map[string]any) {
				u["finalized"].(map[string]any)["created"] = "2001-01-01T00:00:00Z"
			},
			"spec": func(u map[string]any) {
				u["finalized"].(map[string]any)["spec"] = "1.1"
			},
			"content paths": func(u map[string]any) {
				u["finalized"].(map[string]any)["content_paths"].(map[string]any)[digA] = []any{"v4/content/new/c.txt"}
			},
			"new inventory digest": func(u map[string]any) {
				u["finalized"].(map[string]any)["new_inventory_digest"] = strings.Repeat("0", 128)
			},
			"digest algorithm": func(u map[string]any) {
				u["digest_algorithm"] = "sha256"
			},
			"base inventory": func(u map[string]any) {
				u["base_inventory"] = strings.Replace(u["base_inventory"].(string), "Initial import", "Changed", 1)
			},
			"unknown field": func(u map[string]any) {
				u["extra"] = true
			},
		}
		for name, edit := range edits {
			t.Run(name, func(t *testing.T) {
				var fields map[string]any
				be.NilErr(t, json.Unmarshal(saved, &fields))
				edit(fields)
				tampered, err := json.Marshal(fields)
				be.NilErr(t, err)
				var loaded ocfl.ObjectUpdate
				be.Nonzero(t, json.Unmarshal(tampered, &loaded))
			})
		}
	})
}

func TestObjectUpdate_Apply(t *testing.T) {
	ctx := context.Background()
	user := ocfl.User{Name: "Tester"}
	fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `minimal_one_version_one_file`)
	t.Run("draft", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		_, err = upd.Apply(ctx, fsys, "obj", nil)
		be.True(t, errors.Is(err, ocfl.ErrNotFinalized))
		be.True(t, errors.Is(upd.Revert(ctx, fsys, "obj"), ocfl.ErrNotFinalized))
	})
	t.Run("read-only storage", func(t *testing.T) {
		fsys := ocflfs.DirFS(t.TempDir())
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		be.NilErr(t, upd.Finalize("msg", user))
		_, err = upd.Apply(ctx, fsys, "obj", nil)
		be.Nonzero(t, err)
		be.In(t, "does not support writes", err.Error())
	})
	t.Run("returns a new object", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t, fixture)
		obj, err := ocfl.NewObject(ctx, fsys, "minimal_one_version_one_file")
		be.NilErr(t, err)
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new.txt", []byte("new")))
		newObj := commit(t, obj, stage, "v2")
		be.Equal(t, ocfl.V(2), newObj.Head())
		be.Equal(t, obj.Path(), newObj.Path())
		// obj is an out-of-date view
		be.Equal(t, ocfl.V(1), obj.Head())
		// an update from obj conflicts with storage
		stage = ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("other.txt", []byte("other")))
		be.NilErr(t, stage.Update.Finalize("v2", user))
		_, err = stage.Update.Apply(ctx, fsys, obj.Path(), stage.Content)
		be.True(t, errors.Is(err, ocfl.ErrUpdateConflict))
		be.True(t, errors.Is(stage.Update.Revert(ctx, fsys, obj.Path()), ocfl.ErrUpdateConflict))
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, obj.Path()).Err())
	})
	t.Run("applying a committed update again", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t, fixture)
		obj, err := ocfl.NewObject(ctx, fsys, "minimal_one_version_one_file")
		be.NilErr(t, err)
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new.txt", []byte("new")))
		newObj := commit(t, obj, stage, "v2")
		again, err := stage.Update.Apply(ctx, fsys, obj.Path(), nil)
		be.NilErr(t, err)
		be.Equal(t, newObj.InventoryDigest(), again.InventoryDigest())
		be.True(t, errors.Is(stage.Update.Revert(ctx, fsys, obj.Path()), ocfl.ErrRevertUpdate))
	})
	t.Run("wrong object", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t, fixture,
			filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `minimal_no_content`))
		obj, err := ocfl.NewObject(ctx, fsys, "minimal_one_version_one_file")
		be.NilErr(t, err)
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new.txt", []byte("new")))
		be.NilErr(t, stage.Update.Finalize("v2", user))
		_, err = stage.Update.Apply(ctx, fsys, "minimal_no_content", stage.Content)
		be.True(t, errors.Is(err, ocfl.ErrUpdateConflict))
	})
	t.Run("new object over an existing object", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t, fixture)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		be.NilErr(t, upd.Finalize("v1", user))
		_, err = upd.Apply(ctx, fsys, "minimal_one_version_one_file", nil)
		be.True(t, errors.Is(err, ocfl.ErrUpdateConflict))
		be.True(t, errors.Is(upd.Revert(ctx, fsys, "minimal_one_version_one_file"), ocfl.ErrUpdateConflict))
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, "minimal_one_version_one_file").Err())
	})
	t.Run("new object over another update's inventory", func(t *testing.T) {
		// another new object's update was interrupted after writing the root
		// inventory: it isn't ours, so it can't be resumed or reverted.
		fsys := testutil.TmpLocalFS(t)
		other, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		be.NilErr(t, other.Finalize("other v1", user))
		// writes: namaste, version inventory and sidecar, root inventory
		crash := &crashFS{FS: fsys, n: 4}
		_, err = other.Apply(ctx, crash, "obj", nil)
		be.True(t, errors.Is(err, errCrash))
		upd, err := ocfl.NewUpdate(ctx, testutil.TmpLocalFS(t), "obj", "obj")
		be.NilErr(t, err)
		be.NilErr(t, upd.Finalize("v1", user))
		_, err = upd.Apply(ctx, fsys, "obj", nil)
		be.True(t, errors.Is(err, ocfl.ErrUpdateConflict))
		be.True(t, errors.Is(upd.Revert(ctx, fsys, "obj"), ocfl.ErrUpdateConflict))
		// the other update can still be resumed
		_, err = other.Apply(ctx, fsys, "obj", nil)
		be.NilErr(t, err)
	})
	t.Run("new object over a directory that isn't the update's", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		stage := ocfl.NewStage(upd)
		be.NilErr(t, stage.AddBytes("a.txt", []byte("a")))
		be.NilErr(t, upd.Finalize("v1", user))
		// entries in dir that the update doesn't write
		for _, entry := range []string{
			"notes.txt",
			".notes.txt",
			"v2/notes.txt",
			"0=ocfl_object_1.0",
			"inventory.json.sha256",
		} {
			t.Run(entry, func(t *testing.T) {
				_, err := fsys.Write(ctx, path.Join("dir", entry), strings.NewReader("data"))
				be.NilErr(t, err)
				t.Cleanup(func() { be.NilErr(t, fsys.RemoveAll(ctx, "dir")) })
				before := snapshot(t, fsys)
				_, err = upd.Apply(ctx, fsys, "dir", stage.Content)
				be.True(t, errors.Is(err, ocfl.ErrUpdateConflict))
				be.In(t, strings.Split(entry, "/")[0], err.Error())
				be.True(t, errors.Is(upd.Revert(ctx, fsys, "dir"), ocfl.ErrUpdateConflict))
				be.DeepEqual(t, before, snapshot(t, fsys))
			})
		}
	})
	t.Run("new object with leftover temporary files", func(t *testing.T) {
		// a crash while writing a root file can leave the temporary file that
		// local.FS writes it to.
		fsys := testutil.TmpLocalFS(t)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		stage := ocfl.NewStage(upd)
		be.NilErr(t, stage.AddBytes("a.txt", []byte("a")))
		be.NilErr(t, upd.Finalize("v1", user))
		// writes: namaste, content, version inventory and sidecar
		_, err = upd.Apply(ctx, &crashFS{FS: fsys, n: 4}, "obj", stage.Content)
		be.True(t, errors.Is(err, errCrash))
		_, err = fsys.Write(ctx, "obj/.inventory.json.tmp-123", strings.NewReader("partial"))
		be.NilErr(t, err)
		be.NilErr(t, upd.Revert(ctx, fsys, "obj"))
		_, err = ocflfs.ReadDir(ctx, fsys, "obj")
		be.True(t, errors.Is(err, fs.ErrNotExist))
		be.NilErr(t, upd.Finalize("v1", user))
		_, err = fsys.Write(ctx, "obj/.0=ocfl_object_1.1.tmp-123", strings.NewReader("partial"))
		be.NilErr(t, err)
		_, err = upd.Apply(ctx, fsys, "obj", stage.Content)
		be.NilErr(t, err)
	})
	t.Run("missing content", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
		be.NilErr(t, err)
		stage := ocfl.NewStage(upd)
		be.NilErr(t, stage.AddBytes("a.txt", []byte("a")))
		for i := range 8 {
			be.NilErr(t, upd.Add(fmt.Sprintf("missing-%d.txt", i), fmt.Sprintf("%0128d", i), nil))
		}
		be.NilErr(t, upd.Finalize("v1", user))
		_, err = upd.Apply(ctx, fsys, "obj", stage.Content)
		be.True(t, errors.Is(err, ocfl.ErrMissingContent))
		be.In(t, "8 missing digest(s)", err.Error())
		be.In(t, "and 3 more", err.Error())
		_, err = upd.Apply(ctx, fsys, "obj", nil)
		be.True(t, errors.Is(err, ocfl.ErrMissingContent))
		be.In(t, "9 missing digest(s)", err.Error())
		// nothing was written
		_, err = ocflfs.ReadDir(ctx, fsys, "obj")
		be.True(t, errors.Is(err, fs.ErrNotExist))
		// the update can be applied once the content is available
		for i := range 8 {
			stage.Content.AddBytes(fmt.Sprintf("%0128d", i), []byte("not checked"))
		}
		_, err = upd.Apply(ctx, fsys, "obj", stage.Content)
		be.NilErr(t, err)
	})
	t.Run("changed content", func(t *testing.T) {
		// staged files that changed aren't copied: nothing is written until
		// they are restored.
		fsys := testutil.TmpLocalFS(t, fixture)
		obj, err := ocfl.NewObject(ctx, fsys, "minimal_one_version_one_file")
		be.NilErr(t, err)
		contentFS := testutil.TmpLocalFS(t, filepath.Join(`testdata`, `content-fixture`))
		hello := filepath.Join(contentFS.Root(), "content-fixture", "hello.csv")
		helloData, err := os.ReadFile(hello)
		be.NilErr(t, err)
		writeOld(t, hello, string(helloData))
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddFS(ctx, contentFS, "content-fixture", "new"))
		be.NilErr(t, stage.Update.Finalize("v2", user))
		before := snapshot(t, fsys)
		// same size, different content
		be.NilErr(t, os.WriteFile(hello, bytes.Repeat([]byte("x"), len(helloData)), 0o644))
		_, err = stage.Update.Apply(ctx, fsys, obj.Path(), stage.Content)
		be.True(t, errors.Is(err, ocfl.ErrContentChanged))
		be.In(t, `"content-fixture/hello.csv" has changed since it was added`, err.Error())
		be.DeepEqual(t, before, snapshot(t, fsys))
		be.NilErr(t, os.Truncate(hello, 1))
		be.NilErr(t, os.Rename(hello, hello+".moved"))
		_, err = stage.Update.Apply(ctx, fsys, obj.Path(), stage.Content)
		be.True(t, errors.Is(err, ocfl.ErrContentChanged))
		be.In(t, `"content-fixture/hello.csv" is missing`, err.Error())
		be.DeepEqual(t, before, snapshot(t, fsys))
		be.NilErr(t, os.Rename(hello+".moved", hello))
		_, err = stage.Update.Apply(ctx, fsys, obj.Path(), stage.Content)
		be.True(t, errors.Is(err, ocfl.ErrContentChanged))
		be.In(t, `"content-fixture/hello.csv" has size 1, not 15`, err.Error())
		be.DeepEqual(t, before, snapshot(t, fsys))
		// restoring the content changes the file's token, so it is
		// recorded again
		be.NilErr(t, os.WriteFile(hello, helloData, 0o644))
		_, err = stage.Update.Apply(ctx, fsys, obj.Path(), stage.Content)
		be.True(t, errors.Is(err, ocfl.ErrContentChanged))
		readdFile(t, stage.Content, "content-fixture/hello.csv")
		newObj, err := stage.Update.Apply(ctx, fsys, obj.Path(), stage.Content)
		be.NilErr(t, err)
		be.Equal(t, ocfl.V(2), newObj.Head())
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, obj.Path()).Err())
	})
	t.Run("resume with changed content", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t, fixture)
		obj, err := ocfl.NewObject(ctx, fsys, "minimal_one_version_one_file")
		be.NilErr(t, err)
		contentFS := testutil.TmpLocalFS(t, filepath.Join(`testdata`, `content-fixture`))
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddFS(ctx, contentFS, "content-fixture", "new"))
		be.NilErr(t, stage.Update.Finalize("v2", user))
		_, err = stage.Update.Apply(ctx, &crashFS{FS: fsys, n: 1}, obj.Path(), stage.Content)
		be.True(t, errors.Is(err, errCrash))
		_, err = os.Stat(filepath.Join(fsys.Root(), obj.Path(), "v2"))
		be.NilErr(t, err)
		// the interrupted update isn't resumed with changed content
		hello := filepath.Join(contentFS.Root(), "content-fixture", "hello.csv")
		helloData, err := os.ReadFile(hello)
		be.NilErr(t, err)
		be.NilErr(t, os.WriteFile(hello, []byte("changed"), 0o644))
		before := snapshot(t, fsys)
		_, err = stage.Update.Apply(ctx, fsys, obj.Path(), stage.Content)
		be.True(t, errors.Is(err, ocfl.ErrContentChanged))
		be.DeepEqual(t, before, snapshot(t, fsys))
		// it is resumed once the content is restored and recorded again
		be.NilErr(t, os.WriteFile(hello, helloData, 0o644))
		readdFile(t, stage.Content, "content-fixture/hello.csv")
		_, err = stage.Update.Apply(ctx, fsys, obj.Path(), stage.Content)
		be.NilErr(t, err)
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, obj.Path()).Err())
	})
	t.Run("rename without content source", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t, fixture)
		obj, err := ocfl.NewObject(ctx, fsys, "minimal_one_version_one_file")
		be.NilErr(t, err)
		upd := obj.NewUpdate()
		be.NilErr(t, upd.Rename("a_file.txt", "dir/renamed.txt"))
		be.NilErr(t, upd.Finalize("v2", user))
		newObj, err := upd.Apply(ctx, fsys, obj.Path(), nil)
		be.NilErr(t, err)
		be.DeepEqual(t, []string{"dir/renamed.txt"}, newObj.Version(0).State().AllPaths())
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, obj.Path()).Err())
	})
}

// Every fixture can be updated with new content.
func TestObjectUpdate_Fixtures(t *testing.T) {
	ctx := context.Background()
	for _, spec := range []string{`1.0`, `1.1`} {
		fixturesDir := filepath.Join(objectFixturesPath, spec, `good-objects`)
		fixtures, err := os.ReadDir(fixturesDir)
		be.NilErr(t, err)
		for _, dir := range fixtures {
			fixture := filepath.Join(fixturesDir, dir.Name())
			t.Run(fixture, func(t *testing.T) {
				fsys := testutil.TmpLocalFS(t, fixture)
				obj, err := ocfl.NewObject(ctx, fsys, dir.Name())
				be.NilErr(t, err)
				stage := ocfl.NewStage(obj.NewUpdate())
				be.NilErr(t, stage.AddBytes("a-new-file", []byte("new stuff")))
				newObj := commit(t, obj, stage, "update")
				be.NilErr(t, ocfl.ValidateObject(ctx, fsys, newObj.Path()).Err())
				newVersion, err := newObj.VersionFS(ctx, 0)
				be.NilErr(t, err)
				cont, err := fs.ReadFile(newVersion, "a-new-file")
				be.NilErr(t, err)
				be.Equal(t, "new stuff", string(cont))
			})
		}
	}
}

// An update interrupted after any number of writes can be resumed or
// reverted using the saved update.
func TestObjectUpdate_Interrupted(t *testing.T) {
	ctx := context.Background()
	user := ocfl.User{Name: "Tester"}
	type scenario struct {
		// setup creates the object (if any) in fsys, and returns a finalized
		// stage for the update.
		setup    func(t *testing.T, fsys *local.FS) *ocfl.Stage
		baseHead int
	}
	scenarios := map[string]scenario{
		"new object": {
			setup: func(t *testing.T, fsys *local.FS) *ocfl.Stage {
				upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj")
				be.NilErr(t, err)
				stage := stageBytes(t, upd, map[string][]byte{
					"a.txt":     []byte("a"),
					"dir/b.txt": []byte("b"),
					"dir/c.txt": []byte("c"),
				}, digest.MD5)
				be.NilErr(t, upd.Finalize("v1", user))
				return stage
			},
		},
		"existing object with spec upgrade": {
			setup: func(t *testing.T, fsys *local.FS) *ocfl.Stage {
				fixture := filepath.Join(objectFixturesPath, `1.0`, `good-objects`, `spec-ex-full`)
				be.NilErr(t, os.CopyFS(filepath.Join(fsys.Root(), "obj"), os.DirFS(fixture)))
				upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "")
				be.NilErr(t, err)
				stage := ocfl.NewStage(upd)
				be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a")))
				be.NilErr(t, stage.AddBytes("new/b.txt", []byte("b")))
				be.NilErr(t, stage.Rename("foo/bar.xml", "bar.xml"))
				be.NilErr(t, upd.Finalize("v4", user, ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1)))
				return stage
			},
			baseHead: 3,
		},
		"existing sha256 object": {
			setup: func(t *testing.T, fsys *local.FS) *ocfl.Stage {
				upd, err := ocfl.NewUpdate(ctx, fsys, "obj", "obj", ocfl.UpdateWithDigestAlgorithm(digest.SHA256))
				be.NilErr(t, err)
				stage := stageBytes(t, upd, map[string][]byte{
					"a.txt":     []byte("a"),
					"dir/b.txt": []byte("b"),
				})
				be.NilErr(t, upd.Finalize("v1", user))
				_, err = upd.Apply(ctx, fsys, "obj", stage.Content)
				be.NilErr(t, err)
				// v2 keeps sha256: the option is ignored for an existing object
				upd, err = ocfl.NewUpdate(ctx, fsys, "obj", "", ocfl.UpdateWithDigestAlgorithm(digest.SHA512))
				be.NilErr(t, err)
				be.Equal(t, digest.SHA256.ID(), upd.DigestAlgorithm().ID())
				stage = ocfl.NewStage(upd)
				be.NilErr(t, stage.AddBytes("c.txt", []byte("c")))
				be.NilErr(t, stage.Remove("a.txt"))
				be.NilErr(t, upd.Finalize("v2", user))
				return stage
			},
			baseHead: 1,
		},
	}
	for name, sc := range scenarios {
		t.Run(name, func(t *testing.T) {
			// interrupt returns storage with an update interrupted after n
			// writes, a snapshot of the storage before the update, the
			// update's stage and the saved update. done is true if the update
			// completed.
			interrupt := func(t *testing.T, n int) (fsys *local.FS, before map[string]string, stage *ocfl.Stage, saved []byte, done bool) {
				t.Helper()
				fsys = testutil.TmpLocalFS(t)
				stage = sc.setup(t, fsys)
				before = snapshot(t, fsys)
				saved, err := json.Marshal(stage.Update)
				be.NilErr(t, err)
				crash := &crashFS{FS: fsys, n: n}
				_, err = stage.Update.Apply(ctx, crash, "obj", stage.Content, ocfl.UpdateWithGoLimit(1))
				if err == nil {
					return fsys, before, stage, saved, true
				}
				be.True(t, errors.Is(err, errCrash))
				return fsys, before, stage, saved, false
			}
			load := func(t *testing.T, saved []byte) *ocfl.ObjectUpdate {
				t.Helper()
				var u ocfl.ObjectUpdate
				be.NilErr(t, json.Unmarshal(saved, &u))
				return &u
			}
			var n int
			for ; ; n++ {
				fsys, _, stage, saved, done := interrupt(t, n)
				if done {
					break
				}
				t.Run(fmt.Sprintf("resume after %d writes", n), func(t *testing.T) {
					// a new update can't be started over the interrupted one
					_, err := ocfl.NewUpdate(ctx, fsys, "obj", stage.Update.ID())
					if n > 0 {
						be.True(t, errors.Is(err, ocfl.ErrObjectIncomplete))
					}
					obj, err := load(t, saved).Apply(ctx, fsys, "obj", stage.Content)
					be.NilErr(t, err)
					be.Equal(t, sc.baseHead+1, obj.Head().Num())
					be.NilErr(t, ocfl.ValidateObject(ctx, fsys, "obj").Err())
					reopened, err := ocfl.NewObject(ctx, fsys, "obj")
					be.NilErr(t, err)
					be.Equal(t, obj.InventoryDigest(), reopened.InventoryDigest())
				})
				t.Run(fmt.Sprintf("revert after %d writes", n), func(t *testing.T) {
					fsys, before, stage, saved, _ := interrupt(t, n)
					u := load(t, saved)
					// the revert is interrupted too, after m writes: each
					// attempt starts from where the previous one stopped.
					var err error
					for m := 0; ; m++ {
						err = u.Revert(ctx, &crashFS{FS: fsys, n: m}, "obj")
						if !errors.Is(err, errCrash) {
							break
						}
					}
					if errors.Is(err, ocfl.ErrRevertUpdate) {
						// the update was committed: it can only be resumed
						obj, err := u.Apply(ctx, fsys, "obj", stage.Content)
						be.NilErr(t, err)
						be.Equal(t, sc.baseHead+1, obj.Head().Num())
						be.NilErr(t, ocfl.ValidateObject(ctx, fsys, "obj").Err())
						return
					}
					be.NilErr(t, err)
					be.False(t, u.Finalized()) // back to a draft
					_, ok := u.VersionInfo()
					be.False(t, ok)
					be.DeepEqual(t, before, snapshot(t, fsys))
				})
			}
			be.True(t, n > 0)
			t.Logf("%d writes", n)
		})
	}
}

// Steps that fail because the context was canceled aren't logged as errors,
// and steps aren't started after cancellation.
func TestObjectUpdate_Canceled(t *testing.T) {
	user := ocfl.User{Name: "Tester"}
	fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `minimal_one_version_one_file`)
	objPath := path.Base(fixture)
	// setup returns storage with an existing object and a finalized update
	// that adds three files to it.
	setup := func(t *testing.T) (*local.FS, *ocfl.Stage) {
		t.Helper()
		fsys := testutil.TmpLocalFS(t, fixture)
		obj, err := ocfl.NewObject(context.Background(), fsys, objPath)
		be.NilErr(t, err)
		stage := stageBytes(t, obj.NewUpdate(), map[string][]byte{
			"b.txt": []byte("b"),
			"c.txt": []byte("c"),
			"d.txt": []byte("d"),
		})
		be.NilErr(t, stage.Update.Finalize("v2", user))
		return fsys, stage
	}
	// cancelWrites returns fsys with writes to names with the suffix replaced
	// by canceling ctx.
	cancelWrites := func(fsys *local.FS, cancel context.CancelFunc, suffix string) *failFS {
		return &failFS{FS: fsys, fail: func(ctx context.Context, name string) error {
			if !strings.HasSuffix(name, suffix) {
				return nil
			}
			cancel()
			return ctx.Err()
		}}
	}
	t.Run("apply canceled while copying", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fsys, stage := setup(t)
		logs := &logRecorder{}
		_, err := stage.Update.Apply(ctx, cancelWrites(fsys, cancel, "/content/b.txt"), objPath, stage.Content,
			ocfl.UpdateWithGoLimit(1), ocfl.UpdateWithLogger(slog.New(logs)))
		be.True(t, errors.Is(err, context.Canceled))
		// only the copy that was running is logged, and its error isn't
		be.DeepEqual(t, []string{"INFO copy v2/content/b.txt"}, logs.messages(slog.LevelInfo))

		// reverting the interrupted update is canceled too
		ctx, cancel = context.WithCancel(context.Background())
		defer cancel()
		logs = &logRecorder{}
		err = stage.Update.Revert(ctx, cancelWrites(fsys, cancel, objPath+"/inventory.json"), objPath,
			ocfl.UpdateWithLogger(slog.New(logs)))
		be.True(t, errors.Is(err, context.Canceled))
		be.DeepEqual(t, []string{"INFO restore inventory.json"}, logs.messages(slog.LevelInfo))
		be.NilErr(t, stage.Update.Revert(context.Background(), fsys, objPath))
		be.NilErr(t, ocfl.ValidateObject(context.Background(), fsys, objPath).Err())
	})
	t.Run("apply canceled before starting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		fsys, stage := setup(t)
		logs := &logRecorder{}
		_, err := stage.Update.Apply(ctx, fsys, objPath, stage.Content, ocfl.UpdateWithLogger(slog.New(logs)))
		be.True(t, errors.Is(err, context.Canceled))
		be.Zero(t, len(logs.messages(slog.LevelInfo)))
	})
	t.Run("copy fails", func(t *testing.T) {
		ctx := context.Background()
		fsys, stage := setup(t)
		errDiskFull := errors.New("disk full")
		failCopy := &failFS{FS: fsys, fail: func(_ context.Context, name string) error {
			if strings.HasSuffix(name, "/content/b.txt") {
				return errDiskFull
			}
			return nil
		}}
		logs := &logRecorder{}
		_, err := stage.Update.Apply(ctx, failCopy, objPath, stage.Content,
			ocfl.UpdateWithGoLimit(1), ocfl.UpdateWithLogger(slog.New(logs)))
		be.True(t, errors.Is(err, errDiskFull))
		msgs := logs.messages(slog.LevelInfo)
		be.Equal(t, 2, len(msgs))
		be.Equal(t, "INFO copy v2/content/b.txt", msgs[0])
		be.True(t, strings.HasPrefix(msgs[1], "ERROR copy v2/content/b.txt: "))
		be.In(t, errDiskFull.Error(), msgs[1])
	})
}

// crashFS is a local FS that stops writing after n writes or removes,
// simulating an interrupted process.
type crashFS struct {
	*local.FS
	mu sync.Mutex
	n  int
}

var errCrash = errors.New("crashed")

func (c *crashFS) op() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n <= 0 {
		return errCrash
	}
	c.n--
	return nil
}

func (c *crashFS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	if err := c.op(); err != nil {
		return 0, err
	}
	return c.FS.Write(ctx, name, r)
}

func (c *crashFS) Remove(ctx context.Context, name string) error {
	if err := c.op(); err != nil {
		return err
	}
	return c.FS.Remove(ctx, name)
}

func (c *crashFS) RemoveAll(ctx context.Context, name string) error {
	if err := c.op(); err != nil {
		return err
	}
	return c.FS.RemoveAll(ctx, name)
}

// failFS is a local FS whose writes fail with the error returned by fail, if
// it isn't nil.
type failFS struct {
	*local.FS
	fail func(ctx context.Context, name string) error
}

func (f *failFS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	if err := f.fail(ctx, name); err != nil {
		return 0, err
	}
	return f.FS.Write(ctx, name, r)
}

// logRecorder is a [slog.Handler] that records log messages.
type logRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (h *logRecorder) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *logRecorder) WithGroup(string) slog.Handler { return h }

// messages returns the level and message of each record at level or above.
func (h *logRecorder) messages(level slog.Level) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var msgs []string
	for _, r := range h.records {
		if r.Level >= level {
			msgs = append(msgs, r.Level.String()+" "+r.Message)
		}
	}
	return msgs
}

// snapshot returns the paths and contents of all files in fsys.
// readdFile adds the file name in c again, with its current size and content
// token.
func readdFile(t *testing.T, c *ocfl.ContentMap, name string) {
	t.Helper()
	for _, dig := range c.Digests() {
		fsys, srcPath := c.GetContent(dig)
		if srcPath != name {
			continue
		}
		info, err := ocflfs.StatFile(context.Background(), fsys, name)
		be.NilErr(t, err)
		c.AddFile(dig, fsys, name, info)
		return
	}
	t.Fatalf("%q isn't in the content map", name)
}

func snapshot(t *testing.T, fsys *local.FS) map[string]string {
	t.Helper()
	files := map[string]string{}
	dirFS := os.DirFS(fsys.Root())
	err := fs.WalkDir(dirFS, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(dirFS, name)
		files[name] = string(b)
		return err
	})
	be.NilErr(t, err)
	return files
}
