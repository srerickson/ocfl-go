package xfer

import (
	"context"

	"github.com/srerickson/ocfl-go"
	"golang.org/x/sync/errgroup"
)

// transfer dst/src names in files from srcFS to dstFS
func Copy(ctx context.Context, srcFS ocfl.FS, dstFS ocfl.WriteFS, files map[string]string, conc int) error {
	if conc < 1 {
		conc = 1
	}
	grp, ctx := errgroup.WithContext(ctx)
	grp.SetLimit(conc)
	for dst, src := range files {
		dst, src := dst, src
		grp.Go(func() error {
			return copy(ctx, dstFS, dst, srcFS, src)
		})
	}
	return grp.Wait()
}

func copy(ctx context.Context, dstFS ocfl.WriteFS, dst string, srcFS ocfl.FS, src string) error {
	if cpFS, ok := dstFS.(ocfl.CopyFS); ok && dstFS == srcFS {
		return cpFS.Copy(ctx, dst, src)
	}
	// otherwise, manual copy
	srcF, err := srcFS.OpenFile(ctx, src)
	if err != nil {
		return err
	}
	defer srcF.Close()
	if _, err := dstFS.Write(ctx, dst, srcF); err != nil {
		return err
	}
	return nil
}
