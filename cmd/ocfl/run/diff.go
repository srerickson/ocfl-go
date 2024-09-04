package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/cmd/ocfl/internal/diff"
)

type DiffCmd struct {
	ID string `name:"id" short:"i" optional:"" help:"The id for object to diff"`
	Vs []int  `name:"version" short:"v" default:"-1" help:"Object version to compare to HEAD, or object versions to compare if repeated twice. 0 refers to head, negative numbers match versions before HEAD."`
}

func (cmd *DiffCmd) Run(ctx context.Context, root *ocfl.Root, stdout, stderr io.Writer) error {
	obj, err := root.NewObject(ctx, cmd.ID)
	if err != nil {
		return fmt.Errorf("reading object id: %q: %w", cmd.ID, err)
	}
	if !obj.Exists() {
		err := fmt.Errorf("object %q not found at root path %s: %w", cmd.ID, obj.Path(), fs.ErrNotExist)
		return err
	}
	inv := obj.Inventory()
	if inv == nil {
		return errors.New("unexpected nil inventory")
	}
	var v1, v2 int
	switch len(cmd.Vs) {
	case 0:
		return errors.New("missing version numbers to compare")
	case 1:
		// compare version to head
		v1 = cmd.Vs[0]
		v2 = 0
	default:
		v1 = cmd.Vs[0]
		v2 = cmd.Vs[1]
	}
	head := inv.Head().Num()
	if v1 < 0 {
		v1 = head + v1
	}
	if v2 < 0 {
		v2 = head + v2
	}
	var v1Paths, v2Paths ocfl.PathMap
	if v := inv.Version(v1); v != nil && v.State() != nil {
		v1Paths = v.State().PathMap()
	}
	if v := inv.Version(v2); v != nil && v.State() != nil {
		v2Paths = v.State().PathMap()
	}
	if v1Paths == nil {
		return fmt.Errorf("version not found: %d", v1)
	}
	if v2Paths == nil {
		return fmt.Errorf("version not found: %d", v2)
	}
	result, err := diff.Diff(v1Paths, v2Paths)
	if err != nil {
		return err
	}
	if !result.Empty() {
		fmt.Fprint(stdout, result.String())
	}
	return nil
}
