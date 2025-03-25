package server

import (
	"embed"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"time"

	"github.com/srerickson/ocfl-go"
)

var (
	//go:embed templates
	templateFS embed.FS

	templateFuncs = template.FuncMap{
		"objectPath": objectPath,
		"basename":   path.Base,
		"formatDate": formatDate,
	}
)

type OCFLServer struct {
	*http.ServeMux
	root       *ocfl.Root
	index      RootIndex
	indexView  *template.Template
	objectView *template.Template
}

func NewOCFLServer(root *ocfl.Root, index RootIndex) (*OCFLServer, error) {

	indexView, err := template.New("index").Funcs(templateFuncs).ParseFS(templateFS, "templates/base.tmpl.html", "templates/index.tmpl.html")
	if err != nil {
		return nil, err
	}
	objectView, err := template.New("object").Funcs(templateFuncs).ParseFS(templateFS, "templates/base.tmpl.html", "templates/object.tmpl.html")
	if err != nil {
		return nil, err
	}
	srv := &OCFLServer{
		ServeMux:   http.NewServeMux(),
		index:      index,
		root:       root,
		indexView:  indexView,
		objectView: objectView,
	}
	srv.HandleFunc("GET /{$}", srv.indexHandler())
	srv.HandleFunc("GET /object/{id}", srv.objectHanlder())
	srv.HandleFunc("GET /download/{id}/{name}", srv.downloadHandler())
	return srv, nil
}

func (srv *OCFLServer) downloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := r.PathValue("id")
		name := r.PathValue("name")
		if !fs.ValidPath(name) {
			http.Error(w, "invalid file name", http.StatusBadRequest)
			return
		}
		idxObj := srv.index.Get(id)
		if idxObj == nil {
			http.NotFound(w, r)
			return
		}
		fullPath := path.Join(idxObj.Path, name)
		f, err := srv.root.FS().OpenFile(ctx, fullPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Add("Content-Length", strconv.FormatInt(info.Size(), 10))
		if _, err := io.Copy(w, f); err != nil {
			// log error
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func (srv *OCFLServer) indexHandler() http.HandlerFunc {
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

func (srv *OCFLServer) objectHanlder() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := r.PathValue("id")
		var obj *ocfl.Object
		var err error
		switch {
		case srv.root.Layout() == nil:
			idxObj := srv.index.Get(id)
			if idxObj == nil {
				http.NotFound(w, r)
				return
			}
			obj, err = ocfl.NewObject(ctx, srv.root.FS(), idxObj.Path)
		default:
			obj, err = srv.root.NewObject(ctx, id, ocfl.ObjectMustExist())
		}
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		templateData := object{Inventory: obj.Inventory()}
		if err := srv.objectView.ExecuteTemplate(w, "base", &templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
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

func objectPath(id string) string {
	return "/object/" + url.PathEscape(id)
}

func formatDate(t time.Time) string {
	return t.Format(time.DateOnly)
}
