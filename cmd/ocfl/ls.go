package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
)

type LSCmd struct {
	Root        string `name:"root" short:"r" env:"OCFL_ROOT" help:"The prefix/directory of the OCFL storage root used for the command"`
	ID          string `name:"id" short:"i" optional:"" help:"The object ID to list contents from."`
	Version     int    `name:"version" short:"v" default:"0" help:"The object version number (unpadded) to list contents from. The default (0) lists the latest version."`
	WithDigests bool   `name:"digests" short:"d" help:"Show digests when listing contents of an object version."`
}

func (cmd *LSCmd) Run(ctx context.Context, stdout, stderr io.Writer) error {
	ocflv1.Enable() // hopefuly this won't be necessary in the near future.
	fsys, dir, err := parseRootConfig(ctx, cmd.Root)
	if err != nil {
		return err
	}
	root, err := ocfl.NewRoot(ctx, fsys, dir)
	if err != nil {
		return err
	}
	rootCgf := rootConfig(fsys, dir)
	if cmd.ID == "" {
		// list object ids in root
		for obj, err := range root.Objects(ctx) {
			if err != nil {
				return fmt.Errorf("while listing objects in root %q: %w", rootCgf, err)
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
