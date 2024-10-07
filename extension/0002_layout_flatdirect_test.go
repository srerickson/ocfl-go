package extension_test

import "github.com/srerickson/ocfl-go/extension"

var _ (extension.Layout) = (*extension.LayoutFlatDirect)(nil)
var _ (extension.Extension) = (*extension.LayoutFlatDirect)(nil)
