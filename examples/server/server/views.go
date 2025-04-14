package server

import (
	"embed"
	"html/template"
	"net/url"
	"path"
	"time"
)

var (
	//go:embed templates
	templateFS embed.FS
)

type Views struct {
	Index  *template.Template
	Object *template.Template
}

func NewViews() (*Views, error) {
	templateFuncs := template.FuncMap{
		"objectPath":  objectPath,
		"basename":    path.Base,
		"formatDate":  formatDate,
		"shortDigest": shortDigest,
	}

	indexView, err := template.New("index").Funcs(templateFuncs).ParseFS(templateFS, "templates/base.tmpl.html", "templates/index.tmpl.html")
	if err != nil {
		return nil, err
	}
	objectView, err := template.New("object").Funcs(templateFuncs).ParseFS(templateFS, "templates/base.tmpl.html", "templates/object.tmpl.html")
	if err != nil {
		return nil, err
	}
	views := &Views{
		Index:  indexView,
		Object: objectView,
	}
	return views, nil
}

func objectPath(id string) string {
	return "/object/" + url.PathEscape(id)
}

func formatDate(t time.Time) string {
	return t.Format(time.DateOnly)
}

func shortDigest(digest string) string {
	if len(digest) > 8 {
		return digest[0:8]
	}
	return digest
}
