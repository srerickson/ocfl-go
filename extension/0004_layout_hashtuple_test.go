package extension_test

import "github.com/srerickson/ocfl-go/extension"

var _ (extension.Layout) = (*extension.LayoutHashTuple)(nil)
var _ (extension.Extension) = (*extension.LayoutHashTuple)(nil)
