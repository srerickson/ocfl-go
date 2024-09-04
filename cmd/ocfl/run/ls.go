package run

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/srerickson/ocfl-go"
)

const lsHelp = "List objects in a storage root or contents of an object version"

type lsCmd struct {
	ID          string `name:"id" short:"i" optional:"" help:"The id of object to list contents from."`
	Version     int    `name:"version" short:"v" default:"0" help:"The object version number (unpadded) to list contents from. The default (0) lists the latest version."`
	WithDigests bool   `name:"digests" short:"d" help:"Show digests when listing contents of an object version."`
}

func (cmd *lsCmd) Run(ctx context.Context, root *ocfl.Root, stdout, stderr io.Writer) error {
	if cmd.ID == "" {
		// list object ids in root
		for obj, err := range root.Objects(ctx) {
			if err != nil {
				return fmt.Errorf("while listing objects in root: %w", err)
			}
			fmt.Fprintln(stdout, obj.Inventory().ID())
		}
		return nil
	}
	// list contents of an object
	obj, err := root.NewObject(ctx, cmd.ID)
	if err != nil {
		return fmt.Errorf("listing contents from object %q: %w", cmd.ID, err)
	}
	if !obj.Exists() {
		// the object doesn't exist at the expected location
		err := fmt.Errorf("object %q not found at root path %s: %w", cmd.ID, obj.Path(), fs.ErrNotExist)
		return err
	}
	ver := obj.Inventory().Version(cmd.Version)
	if ver == nil {
		err := fmt.Errorf("version %d not found in object %q", cmd.Version, cmd.ID)
		return err
	}
	paths := ver.State().Paths()
	digests := ver.State().PathMap()
	for _, p := range paths {
		if cmd.WithDigests {
			fmt.Fprintln(stdout, digests[p], p)
			continue
		}
		fmt.Fprintln(stdout, p)
	}
	return nil
}
