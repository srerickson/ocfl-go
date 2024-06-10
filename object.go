package ocfl

import (
	"context"
)

var implementations = map[Spec]Implementation{}

type Implementation interface {
	Spec() Spec
	NewObject(context.Context, *ObjectRoot) (Object, error)
}

type Object interface{}

func NewObject(ctx context.Context, fsys FS, dir string, opts ...ObjectOption) (Object, error) {
	obj := &ObjectRoot{
		FS:   fsys,
		Path: dir,
	}
	config := objectOpts{}
	for _, o := range opts {
		o(&config)
	}
	err := obj.ReadRoot(ctx)
	if err != nil {
		obj.stateErr = err
		return obj, obj.stateErr
	}
	return obj, nil
}

type objectOpts struct {
	mustExist    bool
	mustNotExist bool
}

type ObjectOption func(*objectOpts)

func ObjectMustExist() ObjectOption {
	return func(conf *objectOpts) {
		conf.mustExist = true
	}
}

func ObjectMustNotExist() ObjectOption {
	return func(conf *objectOpts) {
		conf.mustNotExist = true
	}
}
