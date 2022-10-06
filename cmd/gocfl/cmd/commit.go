package cmd

import (
	"context"
	"errors"
	"io"

	"github.com/muesli/coral"
	"github.com/spf13/pflag"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
	"github.com/srerickson/ocfl/ocflv1"
)

var commitFlags = struct {
	// srcRepo  string
	newObject bool
	srcPath   string
	dryRun    bool
	objectID  string
	commitMsg string
	userName  string
	userAddr  string
	digestAlg algFlag
}{}

type algFlag struct {
	digest.Alg
}

var _ pflag.Value = (*algFlag)(nil)

func (a *algFlag) Set(id string) error {
	alg, err := digest.NewAlg(id)
	if err != nil {
		return err
	}
	a.Alg = alg
	return nil
}

func (a algFlag) Type() string {
	return "string"
}

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
	commitCmd.Flags().BoolVarP(&commitFlags.newObject, "new", "", false, "creating new object")
	commitCmd.Flags().StringVar(&commitFlags.objectID, "id", "", "Object ID")
	commitCmd.Flags().StringVar(&commitFlags.srcPath, "stage", "", "staging directory for the new object version")
	// commitCmd.Flags().StringVar(&commitFlags.srcRepo, "stage-repo", "", "repo name for staged files")
	commitCmd.Flags().BoolVar(&commitFlags.dryRun, "dry-run", false, "dry run commit. No files are written to the storage root")
	commitCmd.Flags().VarP(&commitFlags.digestAlg, "alg", "", "digest algorithm for new objects (sha512 or sha256). Ignored for updates.")
	commitCmd.Flags().StringVarP(&commitFlags.userAddr, "addr", "a", "", "committer's email address")
	commitCmd.Flags().StringVarP(&commitFlags.userName, "name", "n", "", "committer's name")
	commitCmd.Flags().StringVarP(&commitFlags.commitMsg, "msg", "m", "", "commit message")
	commitCmd.MarkFlagRequired("id")
	commitCmd.MarkFlagRequired("stage")
	commitCmd.MarkFlagRequired("m")
	//commitCmd.Flags().VarPF()
}

func runCommit(ctx context.Context, conf *Config) {
	// fix default flags
	if commitFlags.userAddr == "" {
		commitFlags.userAddr = conf.Email
	}
	if commitFlags.userName == "" {
		commitFlags.userName = conf.Name
	}
	if commitFlags.digestAlg.Alg.ID() == "" {
		commitFlags.digestAlg.Alg = digest.SHA512
	}
	digestAlg := commitFlags.digestAlg.Alg
	// storage root repo
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
	// stage repo
	srcRepo := defaultRepo()
	srcRepo.Path = commitFlags.srcPath
	srcFS, srcRoot, err := srcRepo.GetFSPath(ctx)
	if err != nil {
		log.Error(err, "could not initialize storage driver for staging directory")
	}
	// get store
	storeOpts := &ocflv1.GetStoreConf{
		Logger: log,
	}
	store, err := ocflv1.GetStore(ctx, writeFS, root, storeOpts)
	if err != nil {
		log.Error(err, "can't commit")
		return
	}

	// set digest algorith from exsting object
	var obj *ocflv1.Object
	if !commitFlags.newObject {
		var err error
		obj, err = store.GetObject(ctx, commitFlags.objectID)
		if err != nil {
			log.Error(err, "can't update object")
			return
		}
		inv, err := obj.Inventory(ctx)
		if err != nil {
			log.Error(err, "can't update object")
			return
		}
		digestAlg = inv.DigestAlgorithm
	}

	var index *ocfl.Index
	digestUI := &ProgressWriter{preamble: "computing digests "}
	digestFn := func(w io.Writer) error {
		var err error
		index, err = ocfl.IndexDir(ctx, srcFS, srcRoot, checksum.WithDigest(digestAlg), checksum.WithProgress(w))
		return err
	}
	if err := digestUI.Start(digestFn); err != nil {
		log.Error(err, "staging failed")
		return
	}
	commitUI := &ProgressWriter{preamble: "committing " + commitFlags.objectID + " "}
	commitOpts := []ocflv1.ObjectOption{
		ocflv1.WithAlg(digestAlg),
		ocflv1.WithMessage(commitFlags.commitMsg),
		ocflv1.WithUser(commitFlags.userName, commitFlags.userAddr),
	}
	if commitFlags.dryRun {
		commitOpts = append(commitOpts, ocflv1.WithNoWrite())
	}
	commitFn := func(w io.Writer) error {
		commitOpts = append(commitOpts, ocflv1.WithProgressWriter(w))
		return store.Commit(ctx, commitFlags.objectID, index, commitOpts...)
	}
	if err := commitUI.Start(commitFn); err != nil {
		log.Error(err, "commit failed")
		return
	}
	log.Info("commit complete")
}
