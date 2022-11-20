package extensions

import "errors"

var ErrInvalidLayoutID = errors.New("invalid id for layout")

// Layout is the interface for layout extensions
type Layout interface {
	Extension
	NewFunc() (LayoutFunc, error)
}

// LayoutFunc is a function that maps an object id to a path in the storage root
// or returns an error if the id is invalid
type LayoutFunc func(string) (string, error)
