package server

import (
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"iter"
	"net/http"
	"net/url"

	"github.com/srerickson/ocfl-go"
)

var (
	//go:embed templates
	templateFS embed.FS

	templateFuncs = template.FuncMap{
		"objectPath": objectPath,
	}
)

type server struct {
	*http.ServeMux
	root       *ocfl.Root
	index      RootIndex
	indexView  *template.Template
	objectView *template.Template
}

func NewServer(root *ocfl.Root, index RootIndex) (*server, error) {

	indexView, err := template.New("").Funcs(templateFuncs).ParseFS(templateFS, "templates/base.tmpl.html", "templates/index.tmpl.html")
	if err != nil {
		return nil, err
	}
	objectView, err := template.New("").Funcs(templateFuncs).ParseFS(templateFS, "templates/base.tmpl.html", "templates/object.tmpl.html")
	if err != nil {
		return nil, err
	}
	srv := &server{
		ServeMux:   http.NewServeMux(),
		index:      index,
		root:       root,
		indexView:  indexView,
		objectView: objectView,
	}
	srv.HandleFunc("GET /{$}", srv.indexHandler())
	srv.HandleFunc("GET /object/{id}", srv.objectHanlder())
	return srv, nil
}

func (srv *server) indexHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type templateData struct {
			Objects iter.Seq[*IndexObject]
		}
		data := templateData{Objects: srv.index.Objects()}
		if err := srv.indexView.ExecuteTemplate(w, "base", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (srv *server) objectHanlder() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := r.PathValue("id")
		var (
			obj    *ocfl.Object
			objErr error
		)
		switch {
		case srv.root.Layout() == nil:
			idxObj := srv.index.Get(id)
			if idxObj == nil {
				http.NotFound(w, r)
				return
			}
			obj, objErr = ocfl.NewObject(ctx, srv.root.FS(), idxObj.Path)
		default:
			obj, objErr = srv.root.NewObject(ctx, id, ocfl.ObjectMustExist())
		}
		if objErr != nil {
			if errors.Is(objErr, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, objErr.Error(), http.StatusInternalServerError)
			return
		}
		templateData := object{Inventory: obj.Inventory()}
		if err := srv.objectView.ExecuteTemplate(w, "base", &templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func objectPath(id string) string {
	return "/object/" + url.PathEscape(id)
}

type object struct {
	ocfl.Inventory
}
