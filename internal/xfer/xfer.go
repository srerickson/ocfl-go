package xfer

import (
	"context"
	"errors"
	"io/fs"

	"github.com/srerickson/ocfl-go"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
)

const (
	modeCopy  = "fs-copy"
	modeWrite = "read/write"
)

// transfer dst/src names in files from srcFS to dstFS
func Copy(ctx context.Context, srcFS ocfl.FS, dstFS ocfl.WriteFS, files map[string]string, conc int, logger *slog.Logger) error {
	if conc < 1 {
		conc = 1
	}
	grp, ctx := errgroup.WithContext(ctx)
	grp.SetLimit(conc)
	for dst, src := range files {
		dst, src := dst, src
		grp.Go(func() error {
			return copy(ctx, dstFS, dst, srcFS, src, logger)
		})
	}
	return grp.Wait()
}

func copy(ctx context.Context, dstFS ocfl.WriteFS, dst string, srcFS ocfl.FS, src string, logger *slog.Logger) (err error) {
	xferMode := modeWrite
	cpFS, ok := dstFS.(ocfl.CopyFS)
	if ok && dstFS == srcFS {
		xferMode = modeCopy
	}
	if logger != nil {
		logger.DebugCtx(ctx, "file xfer", "mode", xferMode, "src", src, "dst", dst)
	}
	if xferMode == modeCopy {
		err = cpFS.Copy(ctx, dst, src)
		return
	}
	// otherwise, manual copy
	var srcF fs.File
	srcF, err = srcFS.OpenFile(ctx, src)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := srcF.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	_, err = dstFS.Write(ctx, dst, srcF)
	return err
}
