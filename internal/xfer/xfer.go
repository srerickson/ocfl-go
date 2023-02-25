package xfer

import (
	"context"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/pipeline"
)

type xfer struct {
	gnums int
	algs  []digest.Alg
	src   ocfl.FS
	dst   ocfl.WriteFS
}

type xferItem struct {
	algs []digest.Alg
	src  string
	dst  string
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

func DigestXfer(ctx context.Context, srcFS ocfl.FS, dstFS ocfl.WriteFS, files map[string]string, opts ...Option) (map[string]digest.Set, error) {
	xf := &xfer{
		src: srcFS,
		dst: dstFS,
	}
	for _, o := range opts {
		o(xf)
	}
	setup := func(add func(xferItem) error) error {
		for src, dst := range files {
			item := xferItem{
				algs: xf.algs,
				src:  src,
				dst:  dst,
			}
			if err := add(item); err != nil {
				break
			}
		}
		return nil
	}
	results := make(map[string]digest.Set, len(files))
	resultFn := func(item xferItem, result digest.Set, err error) error {
		if err != nil {
			return err
		}
		results[item.dst] = result
		return nil
	}
	err := pipeline.Run(setup, xf.xferFunc(ctx), resultFn, xf.gnums)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (xf *xfer) xferFunc(ctx context.Context) func(xferItem) (digest.Set, error) {
	return func(item xferItem) (digest.Set, error) {
		srcF, err := xf.src.OpenFile(ctx, item.src)
		if err != nil {
			return nil, err
		}
		defer srcF.Close()
		dig := digest.NewDigester(item.algs...)
		reader := dig.Reader(srcF)
		if _, err := xf.dst.Write(ctx, item.dst, reader); err != nil {
			return nil, err
		}
		return dig.Sums(), nil
	}
}
