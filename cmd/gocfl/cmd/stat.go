package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/muesli/coral"
	"github.com/srerickson/ocfl/ocflv1"
)

var statFlags = struct {
	listObjects bool
	objectID    string
}{}

// statCmd represents the stat command
var statCmd = &coral.Command{
	Use:   "stat",
	Short: "Summary info on storage root or object",
	Long:  "Print useful information about an OCFL storage root or object",
	Run: func(cmd *coral.Command, args []string) {
		conf, err := getConfig()
		if err != nil {
			log.Error("can't load config", "err", err)
			return
		}
		runStat(cmd.Context(), conf)
	},
}

func init() {
	rootCmd.AddCommand(statCmd)
	statCmd.Flags().BoolVarP(&statFlags.listObjects, "list", "l", false, "list all object IDs in the storage root")
	statCmd.Flags().StringVar(&statFlags.objectID, "id", "", "print stats for a single object by its ID")
}

func runStat(ctx context.Context, conf *Config) {
	fsys, root, err := conf.NewFSPath(ctx, rootFlags.repoName)
	if err != nil {
		log.Error("could not initialize storage driver", "repo", rootFlags.repoName, "err", err)
		return
	}
	if closer, ok := fsys.(io.Closer); ok {
		defer closer.Close()
	}
	str, err := ocflv1.GetStore(ctx, fsys, root)
	if err != nil {
		log.Error("could not read storage root", "path", root, "err", err)
		return
	}
	log.Info("storage root info",
		"ocflv", str.Spec(),
		"layout", str.LayoutName(),
		"layout_ok", str.LayoutOK(),
	)
	if statFlags.objectID != "" {
		statObject(ctx, str, statFlags.objectID)
		return
	}
	if statFlags.listObjects {
		log.Info("scanning storage root ...")
		opts := &ocflv1.ScanObjectsOpts{
			Concurrency: 16,
		}

		numObjs := 0
		scanFn := func(obj *ocflv1.Object) error {
			if err := obj.SyncInventory(ctx); err != nil {
				log.Error("can't read object inventory", "err", err)
				return nil
			}
			fmt.Println(obj.Path, ": ", obj.Inventory.ID)
			numObjs++
			return nil
		}
		if err := str.ScanObjects(ctx, scanFn, opts); err != nil {
			log.Error("storage root scan quit with errors", "err", err)
			return
		}
		log.Info("scan complete", "object_count", numObjs)
	}

}

func statObject(ctx context.Context, str *ocflv1.Store, id string) {
	obj, err := str.GetObject(ctx, id)
	if err != nil {
		log.Error("can't read object", "err", err)
		return
	}
	if err := obj.SyncInventory(ctx); err != nil {
		log.Error("can't read object inventory", "err", err)
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(&obj.Inventory); err != nil {
		log.Error(err.Error())
	}

}
