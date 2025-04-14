package server

import (
	"log/slog"
	"net/http"

	"github.com/srerickson/ocfl-go"
)

func NewServer(
	logger *slog.Logger,
	root *ocfl.Root,
	index RootIndex,
	tmpls *Templates,
) http.Handler {
	mux := http.NewServeMux()
	addRoutes(mux, logger, root, index, tmpls)
	return mux
}
