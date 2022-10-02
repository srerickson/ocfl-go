package cmd

import (
	"context"
	"errors"
	"io"

	"github.com/muesli/coral"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/ocflv1"
)

var initRootFlags = struct {
	description string
	layoutName  string
}{}

// statCmd represents the stat command
var initRootCmd = &coral.Command{
	Use:   "init-root",
	Short: "initialize an OCFL storage root",
	Long:  "init initializes the named repo as an OCFL storage root.",
	Run: func(cmd *coral.Command, args []string) {
		conf, err := getConfig()
		if err != nil {
			log.Error(err, "can't load config")
			return
		}
		runInitRoot(cmd.Context(), conf)
	},
}

func init() {
	rootCmd.AddCommand(initRootCmd)
	initRootCmd.Flags().StringVar(&initRootFlags.description, "description", "", "storage root description")
	initRootCmd.Flags().StringVar(&initRootFlags.layoutName, "layout", "", "storage root layout extension")
}

func runInitRoot(ctx context.Context, conf *Config) {
	fsys, root, err := conf.NewFSPath(ctx, rootFlags.repoName)
	if err != nil {
		log.Error(err, "could not initialize storage driver", "repo", rootFlags.repoName)
		return
	}
	if closer, ok := fsys.(io.Closer); ok {
		defer closer.Close()
	}
	writeFS, ok := fsys.(ocfl.WriteFS)
	if !ok {
		err := errors.New("storage driver is read-only")
		log.Error(err, "cannot initialize storage root")
		return
	}
	layoutExt, err := extensions.Get(initRootFlags.layoutName)
	if err != nil {
		log.Error(err, "failed to initialize storage root", "layout", initRootFlags.layoutName)
		return
	}
	layout, ok := layoutExt.(extensions.Layout)
	if !ok {
		err := errors.New("extension is not a layout extension")
		log.Error(err, "failed to initialize storage root", "layout", initRootFlags.layoutName)
		return
	}
	if err := ocflv1.InitStore(ctx, writeFS, root, &ocflv1.InitStoreConf{
		Layout:      layout,
		Description: initRootFlags.description,
	}); err != nil {
		log.Error(err, "during storage root initialization")
		return
	}
	runStat(ctx, conf)
}
