package cmd

import (
	"context"
	"errors"
	"io"

	"github.com/muesli/coral"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
)

var commitFlags = struct {
	srcRepo  string
	srcPath  string
	dryRun   bool
	objectID string
}{}

var commitCmd = &coral.Command{
	Use:   "commit",
	Short: "create or update objects in the storage root",
	Long:  "commit creates new object versions or updates existing objects",
	Run: func(cmd *coral.Command, args []string) {
		conf, err := getConfig()
		if err != nil {
			log.Error(err, "can't load config")
			return
		}
		runCommit(cmd.Context(), conf)
	},
}

func init() {
	rootCmd.AddCommand(commitCmd)
	commitCmd.Flags().StringVar(&commitFlags.objectID, "id", "", "Object ID")
	commitCmd.Flags().StringVar(&commitFlags.srcPath, "stage", "", "staging directory for version files")
	commitCmd.Flags().StringVar(&commitFlags.srcRepo, "stage-repo", "", "repo name for staged files")
}

func runCommit(ctx context.Context, conf *Config) {
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
	// FIXME: What is the best way to configure the staging area
	// TODO: this can be cleaner...
	srcRepo := conf.Repo(commitFlags.srcRepo, false)
	if srcRepo == nil {
		srcRepo = defaultRepo()
	}
	srcRepo.Path = commitFlags.srcPath
	srcFS, srcRoot, err := srcRepo.GetFSPath(ctx)
	if err != nil {
		log.Error(err, "could not initialize storage driver for staging directory")
	}
	store, err := ocflv1.GetStore(ctx, writeFS, root, nil)
	if err != nil {
		log.Error(err, "can't commit")
		return
	}
	stage, err := store.StageNew(ctx, commitFlags.objectID, ocflv1.StageFS(srcFS))
	if err != nil {
		log.Error(err, "can't commit")
		return
	}
	digestUI := &ProgressWriter{preamble: "computing digests "}
	digestFn := func(w io.Writer) error { return stage.AddDir(ctx, srcRoot, ocflv1.StageAddProgress(w)) }
	if err := digestUI.Start(digestFn); err != nil {
		log.Error(err, "staging failed")
		return
	}
	commitUI := &ProgressWriter{preamble: "committing " + commitFlags.objectID + " "}
	commitFn := func(w io.Writer) error { return store.Commit(ctx, stage, ocflv1.CommitProgress(w)) }
	if err := commitUI.Start(commitFn); err != nil {
		log.Error(err, "commit failed")
		return
	}
	log.Info("commit complete")
}
