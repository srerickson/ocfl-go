package main

import (
	"context"
	"fmt"
	"io"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/ocflv1"
)

type InitRootCmd struct {
	Layout      string `name:"layout" short:"l" optional:"" default:"0004-hashed-n-tuple-storage-layout"  help:"The storage root layout extension (see https://ocfl.github.io/extensions/)."`
	Description string `name:"description" short:"d" optional:"" help:"Description to include in the storage root metadata"`
	Spec        string `name:"ocflv" default:"1.1" help:"OCFL version for the storage root"`
	Path        string `arg:"" name:"path" help:"Local directory or S3 bucket/prefix where the storage root will be initialized"`
}

func (cmd *InitRootCmd) Run(ctx context.Context, stdout, stderr io.Writer) error {
	ocflv1.Enable() // hopefuly this won't be necessary in the near future.
	fsys, dir, err := parseRootConfig(ctx, cmd.Path)
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
	_, err = ocfl.NewRoot(ctx, fsys, dir, ocfl.InitRoot(spec, cmd.Description, layout))
	if err != nil {
		return fmt.Errorf("failed to initialize new root at %s: $w", err)
	}
	rootCfg := rootConfig(fsys, dir)
	fmt.Println("storage root:", rootCfg)
	fmt.Println("layout:", layout.Name())
	fmt.Printf("To use the storage root with other commands, set:\n$ export OCFL_ROOT=%s\n", rootCfg)
	return nil
}
