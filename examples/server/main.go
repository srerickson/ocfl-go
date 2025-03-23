package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

//go:embed templates
var templateFS embed.FS

type Index struct {
	Objects []IndexObject
}

type IndexObject struct {
	ID          string
	Head        string
	LastUpdated time.Time
}

func (obj IndexObject) Path() string {
	return "/object/" + url.PathEscape(obj.ID)
}

func main() {
	ctx := context.Background()
	flag.Parse()
	rootPath := flag.Arg(0)
	if err := server(ctx, rootPath); err != nil {
		log.Fatal(err)
	}

}

func server(ctx context.Context, rootPath string) error {
	indexView, err := template.ParseFS(templateFS, "templates/base.tmpl.html", "templates/index.tmpl.html")
	if err != nil {
		return err
	}
	objectView, err := template.ParseFS(templateFS, "templates/base.tmpl.html", "templates/object.tmpl.html")
	if err != nil {
		return err
	}

	fsys := ocflfs.DirFS(rootPath)
	root, err := ocfl.NewRoot(ctx, fsys, ".")
	if err != nil {
		return err
	}

	var index Index
	for obj := range root.Objects(ctx) {
		if err != nil {
			return err
		}
		index.Objects = append(index.Objects, IndexObject{
			ID:          obj.ID(),
			Head:        obj.Inventory().Head().String(),
			LastUpdated: obj.Inventory().Version(0).Created(),
		})
	}

	mx := http.NewServeMux()
	mx.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if err := indexView.ExecuteTemplate(w, "base", &index); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	mx.HandleFunc("GET /object/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		obj, err := root.NewObject(ctx, id, ocfl.ObjectMustExist())
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := objectView.ExecuteTemplate(w, "base", obj.Inventory()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	return http.ListenAndServe(":8877", mx)
}
