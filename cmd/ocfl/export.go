package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
)

type ExportCmd struct {
	ID      string `name:"id" short:"i" help:"The ID for the object to export"`
	Root    string `name:"root" short:"r" env:"OCFL_ROOT" help:"The prefix/directory of the OCFL storage root for the object"`
	Version int    `name:"version" short:"v" default:"0" help:"The object version number (unpadded) to list contents from. The default (0) lists the latest version."`
	Src     string `name:"src" short:"s" default:"." help:"The source path of the object content to export"`
	Replace bool   `name:"replace" help:"replace existing files with object contents"`
	Dst     string `arg:"" name:"dst" help:"The destination path where exported content will be saved."`
}

func (cmd *ExportCmd) Run(ctx context.Context, stdout, stderr io.Writer) error {
	ocflv1.Enable() // hopefuly this won't be necessary in the near future.
	fsys, dir, err := parseRootConfig(ctx, cmd.Root)
	if err != nil {
		return err
	}
	root, err := ocfl.NewRoot(ctx, fsys, dir)
	if err != nil {
		return err
	}
	// rootCgf := rootConfig(fsys, dir)

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
	logical, err := obj.OpenVersion(ctx, cmd.Version)
	if err != nil {
		return err
	}
	logicalState := logical.State()
	if logicalState == nil {
		return errors.New("encountered unexpected nil state")
	}
	if logicalState.GetDigest(cmd.Src) != "" {
		// explicit source file export
		if cmd.Dst == "-" {
			return exportFile(logical, cmd.Src, cmd.Replace, stdout)
		}
		return exportFile(logical, cmd.Src, cmd.Replace, nil, cmd.Dst)
	}
	if cmd.Dst == "-" {
		return errors.New("export to stdout requires src to be a single file")
	}
	// treat cmd.Src is the parent directory for the content to export
	exports := map[string][]string{} // srcname -> dstNames
	for _, logicalNames := range logicalState {
		if len(logicalNames) < 1 {
			return errors.New("version state is corrupt")
		}
		srcName := logicalNames[0]
		for _, logicalName := range logicalNames {
			if cmd.Src == "." {
				dstName := filepath.Join(cmd.Dst, filepath.FromSlash(logicalName))
				exports[srcName] = append(exports[srcName], dstName)
				continue
			}
			if strings.HasPrefix(logicalName, cmd.Src+"/") {
				dstName := filepath.Join(cmd.Dst, filepath.FromSlash(strings.TrimPrefix(logicalName, cmd.Src+"/")))
				exports[srcName] = append(exports[srcName], dstName)
			}
		}
	}
	for srcName, dstNames := range exports {
		if err := exportFile(logical, srcName, cmd.Replace, nil, dstNames...); err != nil {
			return err
		}
	}
	return nil
}

func exportFile(srcFS fs.FS, srcName string, replace bool, stdout io.Writer, dstNames ...string) (err error) {
	f, err := srcFS.Open(srcName)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if stdout != nil {
		_, err = io.Copy(stdout, f)
		return
	}
	const FileMode, DirMode fs.FileMode = 0664, 0775
	perm := os.O_WRONLY | os.O_CREATE
	switch {
	case replace:
		// replace file if it exists
		perm |= os.O_TRUNC
	default:
		// file must not exist
		perm |= os.O_EXCL
	}
	writers := make([]io.Writer, len(dstNames))
	for i, name := range dstNames {
		var f *os.File
		if err = os.MkdirAll(filepath.Dir(name), DirMode); err != nil {
			return
		}
		f, err = os.OpenFile(name, perm, FileMode)
		if err != nil {
			return
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()
		writers[i] = f
	}
	_, err = io.Copy(io.MultiWriter(writers...), f)
	return
}
