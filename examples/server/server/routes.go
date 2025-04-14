package server

import (
	"errors"
	"html/template"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"net/http"
	"path"
	"strconv"

	"github.com/srerickson/ocfl-go"
)

func addRoutes(
	mux *http.ServeMux,
	logger *slog.Logger,
	root *ocfl.Root,
	index RootIndex,
	indexView *template.Template,
	objectView *template.Template,

) {
	mux.HandleFunc("GET /{$}", handleIndex(index, indexView))
	mux.HandleFunc("GET /object/{id}", handleObject(logger, root, index, objectView))
	mux.HandleFunc("GET /download/{id}/{name}", handleDownload(logger, root, index))
}

func handleDownload(logger *slog.Logger, root *ocfl.Root, index RootIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := r.PathValue("id")
		name := r.PathValue("name")
		if !fs.ValidPath(name) {
			http.Error(w, "invalid file name", http.StatusBadRequest)
			return
		}
		idxObj := index.Get(id)
		if idxObj == nil {
			http.NotFound(w, r)
			return
		}
		fullPath := path.Join(idxObj.Path, name)
		f, err := root.FS().OpenFile(ctx, fullPath)
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
			logger.Error(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func handleIndex(index RootIndex, view *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type templateData struct {
			Objects iter.Seq[*IndexObject]
		}
		data := templateData{Objects: index.Objects()}
		if err := view.ExecuteTemplate(w, "base", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func handleObject(logger *slog.Logger, root *ocfl.Root, index RootIndex, view *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := r.PathValue("id")
		var obj *ocfl.Object
		var err error
		switch {
		case root.Layout() == nil:
			idxObj := index.Get(id)
			if idxObj == nil {
				http.NotFound(w, r)
				return
			}
			obj, err = ocfl.NewObject(ctx, root.FS(), idxObj.Path)
		default:
			obj, err = root.NewObject(ctx, id, ocfl.ObjectMustExist())
		}
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			logger.Error(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		templateData := object{Inventory: obj.Inventory()}
		if err := view.ExecuteTemplate(w, "base", &templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
