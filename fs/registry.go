package fs

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
)

// ErrUnknownScheme is returned by [Registry.Open] for a configuration string
// with a URL scheme that isn't in the registry.
var ErrUnknownScheme = errors.New("unrecognized fs configuration scheme")

// OpenFunc opens an FS from a configuration string, such as the text returned
// by an FS's MarshalText method (see [encoding.TextMarshaler]).
type OpenFunc func(ctx context.Context, conf string) (FS, error)

// Registry is an immutable collection of functions that open an FS from a
// configuration string, indexed by URL scheme. The zero value is an empty
// registry.
//
// This package has no default registry, since it can't import the storage
// backends that import it. The fs/config package provides one for the
// backends in this module: [config.Registry].
//
// [config.Registry]: https://pkg.go.dev/github.com/srerickson/ocfl-go/fs/config#Registry
type Registry struct {
	openers map[string]OpenFunc // lowercase scheme -> opener
}

// NewRegistry returns a Registry with the given openers, indexed by URL
// scheme. Schemes are case-insensitive.
func NewRegistry(openers map[string]OpenFunc) Registry {
	r := Registry{openers: make(map[string]OpenFunc, len(openers))}
	for scheme, open := range openers {
		r.openers[strings.ToLower(scheme)] = open
	}
	return r
}

// Append returns a new Registry with the openers from r plus open for scheme.
// If r already has an opener for scheme, the new registry uses open.
func (r Registry) Append(scheme string, open OpenFunc) Registry {
	newR := Registry{openers: maps.Clone(r.openers)}
	if newR.openers == nil {
		newR.openers = map[string]OpenFunc{}
	}
	newR.openers[strings.ToLower(scheme)] = open
	return newR
}

// Open opens the FS described by conf with the opener for conf's URL scheme.
// If r has no opener for the scheme, it returns an error wrapping
// [ErrUnknownScheme].
func (r Registry) Open(ctx context.Context, conf string) (FS, error) {
	u, err := url.Parse(conf)
	if err != nil {
		return nil, fmt.Errorf("parsing fs configuration: %w", err)
	}
	open, ok := r.openers[u.Scheme] // url.Parse lowercases the scheme
	if !ok {
		return nil, fmt.Errorf("fs configuration %q: %w: %q", u.Redacted(), ErrUnknownScheme, u.Scheme)
	}
	return open(ctx, conf)
}

// Schemes returns the sorted URL schemes in r.
func (r Registry) Schemes() []string {
	return slices.Sorted(maps.Keys(r.openers))
}
