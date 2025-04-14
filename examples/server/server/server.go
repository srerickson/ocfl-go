package server

import (
	"html/template"
	"iter"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

	"github.com/srerickson/ocfl-go"
)

func NewServer(
	logger *slog.Logger,
	root *ocfl.Root,
	index RootIndex,
	indexView *template.Template,
	objectView *template.Template,

) http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, logger, root, index, indexView, objectView)
	return mux
}

type object struct {
	ocfl.Inventory
}

func (o object) DownloadPath(digest string) string {
	manifest := o.Manifest()
	if manifest == nil {
		return ""
	}
	paths := manifest[digest]
	if len(paths) < 1 {
		return ""
	}
	return "/download/" + url.PathEscape(o.ID()) + "/" + url.PathEscape(paths[0])
}

// iterate over versions in order or preesntation (reversed)
func (o object) Versions() iter.Seq2[string, ocfl.ObjectVersion] {
	return func(yield func(string, ocfl.ObjectVersion) bool) {
		vers := o.Head().Lineage()
		slices.Reverse(vers)
		for _, v := range vers {
			if !yield(v.String(), o.Version(v.Num())) {
				return
			}
		}
	}
}
