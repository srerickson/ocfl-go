package run

import (
	"context"
	"fmt"
	"io"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/extension"
)

const initRootHelp = `Create a new OCFL storage root`

type initRootCmd struct {
	Layout      string `name:"layout" short:"l" optional:"" default:"0004-hashed-n-tuple-storage-layout"  help:"The storage root layout extension (see https://ocfl.github.io/extensions/)."`
	Description string `name:"description" short:"d" optional:"" help:"Description to include in the storage root metadata"`
	Spec        string `name:"ocflv" default:"1.1" help:"OCFL version for the storage root"`
}

func (cmd *initRootCmd) Run(ctx context.Context, fsysConfig string, stdout, stderr io.Writer) error {
	fsys, dir, err := parseRootConfig(ctx, fsysConfig)
	if err != nil {
		return err
	}
	spec := ocfl.Spec(cmd.Spec)
	layout, err := extension.Get(cmd.Layout)
	if err != nil {
		return fmt.Errorf("%q: %w", cmd.Layout, err)
	}
	_, isLayout := layout.(extension.Layout)
	if !isLayout {
		return fmt.Errorf("%s: %w", cmd.Layout, extension.ErrNotLayout)
	}
	root, err := ocfl.NewRoot(ctx, fsys, dir, ocfl.InitRoot(spec, cmd.Description, layout))
	if err != nil {
		return fmt.Errorf("failed to initialize new root at %s: $w", err)
	}
	rootCfg := rootConfig(fsys, dir)
	fmt.Fprintln(stdout, "storage root:", rootCfg)
	if l := root.LayoutName(); l != "" {
		fmt.Fprintln(stdout, "layout:", root.LayoutName())
	}
	if d := root.Description(); d != "" {
		fmt.Fprintln(stdout, "description:", root.Description())
	}
	fmt.Fprintln(stdout, "OCFL version:", root.Spec())
	return nil
}
