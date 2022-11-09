package cmd

import (
	"context"
	"errors"
	"io"
	"path"

	"github.com/muesli/coral"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
)

var removeFlags = struct {
	isPath bool
}{}

var removeCmd = &coral.Command{
	Use:   "remove [-p] arg",
	Short: "remove an object or path in a storage root",
	Long:  "remove an object or path in a storage root",
	Run: func(cmd *coral.Command, args []string) {
		conf, err := getConfig()
		if err != nil {
			log.Error(err, "can't load config")
			return
		}
		runRemove(cmd.Context(), conf, removeFlags.isPath, args[0])
	},
	Args: coral.ExactArgs(1),
}

func init() {
	rootCmd.AddCommand(removeCmd)
	removeCmd.Flags().BoolVarP(&removeFlags.isPath, "path", "p", false, "argument is a storage root path")
}

func runRemove(ctx context.Context, conf *Config, isPath bool, arg string) {
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
	store, err := ocflv1.GetStore(ctx, writeFS, root)
	if err != nil {
		log.Error(err, "can't connect to storage root")
		return
	}
	rmPath := arg
	if !isPath {
		objPath, err := store.ResolveID(arg)
		if err != nil {
			log.Error(err, "can't resolve path for object")
			return
		}
		rmPath = path.Join(root, objPath)
	}
	if err := writeFS.RemoveAll(ctx, rmPath); err != nil {
		log.Error(err, "error during remove")
		return
	}
	log.Info("removed", "dir", rmPath)
}
