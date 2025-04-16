package server

import (
	"embed"
	"fmt"
	"html/template"
	"net/url"
	"path"
	"time"
)

var (
	//go:embed templates
	templateFS embed.FS

	templateFuncs = template.FuncMap{
		"objectPath":    objectPath,
		"downloadPath":  downloadPath,
		"basename":      path.Base,
		"formatDate":    formatDate,
		"shortDigest":   shortDigest,
		"humanFileSize": humanFileSize,
	}
)

type Templates struct {
	Index  *template.Template
	Object *template.Template
}

func ReadTemplates() (*Templates, error) {
	indexView, err := template.New("index").Funcs(templateFuncs).ParseFS(templateFS, "templates/base.tmpl.html", "templates/index.tmpl.html")
	if err != nil {
		return nil, err
	}
	objectView, err := template.New("object").Funcs(templateFuncs).ParseFS(templateFS, "templates/base.tmpl.html", "templates/object.tmpl.html")
	if err != nil {
		return nil, err
	}
	views := &Templates{
		Index:  indexView,
		Object: objectView,
	}
	return views, nil
}

func objectPath(id string) string {
	if id == "" {
		return ""
	}
	return "/object/" + url.PathEscape(id)
}

func downloadPath(id string, contentPath string) string {
	if contentPath == "" || id == "" {
		return ""
	}
	return "/download/" + url.PathEscape(id) + "/" + url.PathEscape(contentPath)
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

func humanFileSize(byteSize int64) string {
	var units = []string{"Bytes", "KB", "MB", "GB", "TB"}
	scaled := float64(byteSize)
	unit := ""
	for i := 0; i < len(units); i++ {
		unit = units[i]
		if scaled < 1000 {
			break
		}
		scaled = scaled / 1000
	}
	if unit == "Bytes" {
		return fmt.Sprintf("%d %s", int64(scaled), unit)
	}
	return fmt.Sprintf("%0.2f %s", scaled, unit)
}
