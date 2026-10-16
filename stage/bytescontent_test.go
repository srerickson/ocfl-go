package stage_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/stage"
)

func TestBytesContentMap(t *testing.T) {
	ctx := context.Background()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	t.Run("zero value", func(t *testing.T) {
		var c stage.BytesContentMap
		srcFS, srcPath := c.GetContent(digA)
		be.Zero(t, srcFS)
		be.Zero(t, srcPath)
		be.Equal(t, 0, len(c.Digests()))
		c.Remove(digA)
		c.DeleteFunc(func(string) bool { return true })
	})
	t.Run("bytes", func(t *testing.T) {
		var c stage.BytesContentMap
		c.StageBytes(strings.ToUpper(digA), []byte("a"))
		c.StageBytes(digB, []byte("b"))
		be.DeepEqual(t, []string{digA, digB}, c.Digests())
		srcFS, srcPath := c.GetContent(strings.ToUpper(digA))
		got, err := ocflfs.ReadAll(ctx, srcFS, srcPath)
		be.NilErr(t, err)
		be.Equal(t, "a", string(got))
		// replace content
		c.StageBytes(digB, []byte("bb"))
		srcFS, srcPath = c.GetContent(digB)
		got, err = ocflfs.ReadAll(ctx, srcFS, srcPath)
		be.NilErr(t, err)
		be.Equal(t, "bb", string(got))
		c.Remove(strings.ToUpper(digA))
		be.DeepEqual(t, []string{digB}, c.Digests())
		srcFS, _ = c.GetContent(digA)
		be.Zero(t, srcFS)
		c.DeleteFunc(func(dig string) bool { return dig == digB })
		be.Equal(t, 0, len(c.Digests()))
	})
}

func TestBytesContentMap_DeleteFunc(t *testing.T) {
	// content isn't removed when the update stops needing it: it does no
	// harm, and DeleteFunc with NeedsContent removes it.
	ctx := context.Background()
	fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `spec-ex-full`)
	newUpdate := func(t *testing.T) *ocfl.ObjectUpdate {
		u, _, err := ocfl.NewObjectUpdate(ctx, ocflfs.DirFS(fixture), ".", "")
		be.NilErr(t, err)
		return u
	}
	prune := func(u *ocfl.ObjectUpdate, c *stage.BytesContentMap) {
		c.DeleteFunc(func(dig string) bool { return !u.NeedsContent(dig) })
	}
	t.Run("remove", func(t *testing.T) {
		u := newUpdate(t)
		var c stage.BytesContentMap
		be.NilErr(t, stage.Bytes(u, &c, "new/a.txt", []byte("a")))
		be.NilErr(t, stage.Bytes(u, &c, "new/b.txt", []byte("a")))
		dig := u.NewState().DigestFor("new/a.txt")
		be.NilErr(t, u.Remove("new/a.txt"))
		// content is still used by new/b.txt
		prune(u, &c)
		be.DeepEqual(t, []string{dig}, c.Digests())
		be.NilErr(t, u.Remove("new"))
		be.DeepEqual(t, []string{dig}, c.Digests())
		prune(u, &c)
		be.Equal(t, 0, len(c.Digests()))
	})
	t.Run("clear", func(t *testing.T) {
		u := newUpdate(t)
		var c stage.BytesContentMap
		be.NilErr(t, stage.Bytes(u, &c, "new/a.txt", []byte("a")))
		be.NilErr(t, u.Clear())
		prune(u, &c)
		be.Equal(t, 0, len(c.Digests()))
		// the content can be added again
		be.NilErr(t, stage.Bytes(u, &c, "new/a.txt", []byte("a")))
		be.Equal(t, 1, len(c.Digests()))
	})
	t.Run("rename keeps content", func(t *testing.T) {
		u := newUpdate(t)
		var c stage.BytesContentMap
		be.NilErr(t, stage.Bytes(u, &c, "new/a.txt", []byte("a")))
		be.NilErr(t, u.Rename("new", "newer"))
		prune(u, &c)
		be.DeepEqual(t, []string{u.NewState().DigestFor("newer/a.txt")}, c.Digests())
	})
}
