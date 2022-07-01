package logger

import (
	"os"

	"github.com/go-logr/logr"
	"github.com/iand/logfmtr"
)

var opts = logfmtr.Options{
	Writer:    os.Stderr,
	Colorize:  true,
	Humanize:  true,
	NameDelim: "/",
}
var defaultLogger = logfmtr.NewWithOptions(opts)

func DefaultLogger() logr.Logger {
	return defaultLogger
}
