package xfer

import (
	"context"
	"io"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/pipeline"
)

type xfer struct {
	gnums    int
	algs     []digest.Alg
	progress io.Writer
	src      ocfl.FS
	dst      ocfl.WriteFS
}

type xferItem struct {
	algs   []digest.Alg
	src    string
	dst    string
	result digest.Set
	err    error
}

type Option func(*xfer)

func WithGoNums(i int) Option {
	return func(x *xfer) {
		x.gnums = i
	}
}

func WithAlgs(algs ...digest.Alg) Option {
	return func(x *xfer) {
		x.algs = algs
	}
}

func WithProgress(p io.Writer) Option {
	return func(x *xfer) {
		x.progress = p
	}
}

func DigestXfer(ctx context.Context, srcFS ocfl.FS, dstFS ocfl.WriteFS, files map[string]string, opts ...Option) (map[string]digest.Set, error) {
	xf := &xfer{
		src: srcFS,
		dst: dstFS,
	}
	for _, o := range opts {
		o(xf)
	}
	setup := func(add func(*xferItem) error) error {
		for src, dst := range files {
			item := &xferItem{
				algs: xf.algs,
				src:  src,
				dst:  dst,
			}
			if err := add(item); err != nil {
				return err
			}
		}
		return nil
	}
	results := make(map[string]digest.Set, len(files))
	resultFn := func(item *xferItem) error {
		if item.err != nil {
			return item.err
		}
		results[item.dst] = item.result
		return nil
	}
	err := pipeline.Run(ctx, setup, xf.xferFunc(), resultFn, xf.gnums)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (xf *xfer) xferFunc() func(context.Context, *xferItem) (*xferItem, error) {
	return func(ctx context.Context, item *xferItem) (*xferItem, error) {
		srcF, err := xf.src.OpenFile(ctx, item.src)
		if err != nil {
			item.err = err
			return item, err
		}
		defer func() {
			if err := srcF.Close(); err != nil && item.err == nil {
				item.err = err
			}
		}()
		dig := digest.NewDigester(item.algs...)
		reader := dig.Reader(srcF)
		if xf.progress != nil {
			reader = io.TeeReader(reader, xf.progress)
		}
		if _, err := xf.dst.Write(ctx, item.dst, reader); err != nil {
			item.err = err
			return item, item.err
		}
		item.result = dig.Sums()
		return item, item.err
	}
}
