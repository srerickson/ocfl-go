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
			log.Error(err, "can't load config")
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
		log.Error(err, "could not initialize storage driver", "repo", rootFlags.repoName)
		return
	}
	if closer, ok := fsys.(io.Closer); ok {
		defer closer.Close()
	}
	str, err := ocflv1.GetStore(ctx, fsys, root, nil)
	if err != nil {
		log.Error(err, "could not read storage root", "path", root)
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
		scan, err := str.ScanObjects(ctx, opts)
		if err != nil {
			log.Error(err, "storage root scan quit with errors")
			return
		}
		log.Info("scan complete", "object_count", len(scan))
		for p := range scan {
			obj, err := str.GetObjectPath(ctx, p)
			if err != nil {
				log.Error(err, "can't read object")
				return
			}
			inv, err := obj.Inventory(ctx)
			if err != nil {
				log.Error(err, "can't read object inventory")
				return
			}
			fmt.Println(inv.ID)
		}
	}

}

func statObject(ctx context.Context, str *ocflv1.Store, id string) {
	obj, err := str.GetObject(ctx, id)
	if err != nil {
		log.Error(err, "can't read object")
		return
	}
	inv, err := obj.Inventory(ctx)
	if err != nil {
		log.Error(err, "can't read object inventory")
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(&inv); err != nil {
		log.Error(err, "print stats")
	}

}
