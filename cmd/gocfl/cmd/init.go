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
			log.Error("can't load config", "err", err)
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
		log.Error("could not initialize storage driver", "repo", rootFlags.repoName, "err", err)
		return
	}
	if closer, ok := fsys.(io.Closer); ok {
		defer closer.Close()
	}
	writeFS, ok := fsys.(ocfl.WriteFS)
	if !ok {
		err := errors.New("storage driver is read-only")
		log.Error("cannot initialize storage root", "err", err)
		return
	}
	var layout extensions.Layout
	if initRootFlags.layoutName != "" {
		layoutExt, err := extensions.Get(initRootFlags.layoutName)
		if err != nil {
			log.Error("can't initialize storage root with layout", "layout", initRootFlags.layoutName, "err", err)
			return
		}
		layout, ok = layoutExt.(extensions.Layout)
		if !ok {
			err := errors.New("extension is not a layout extension")
			log.Error("can't initialize storage root with layout", "layout", initRootFlags.layoutName, "err", err)
			return
		}
	}
	if err := ocflv1.InitStore(ctx, writeFS, root, &ocflv1.InitStoreConf{
		Layout:      layout,
		Description: initRootFlags.description,
	}); err != nil {
		log.Error("during storage root initialization", "err", err)
		return
	}
	runStat(ctx, conf)
}
