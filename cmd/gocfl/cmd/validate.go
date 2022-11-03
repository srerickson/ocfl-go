package cmd

import (
	"context"
	"io"
	"path"

	"github.com/muesli/coral"
	"github.com/srerickson/ocfl/ocflv1"
)

var validateFlags = struct {
	objectID string
}{}

// validateCmd represents the validate command
var validateCmd = &coral.Command{
	Use:   "validate",
	Short: "Validates an OCFL Object or Storage Root",
	Long:  `Validates an OCFL Object or Storage Root`,
	Run: func(cmd *coral.Command, args []string) {
		conf, err := getConfig()
		if err != nil {
			log.Error(err, "can't load config")
			return
		}
		runValidate(cmd.Context(), conf)
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().StringVar(&validateFlags.objectID, "id", "", "ID of object to validate")
}

func runValidate(ctx context.Context, conf *Config) {
	fsys, root, err := conf.NewFSPath(ctx, rootFlags.repoName)
	if err != nil {
		log.Error(err, "could not initialize storage driver", "repo", rootFlags.repoName)
		return
	}
	if closer, ok := fsys.(io.Closer); ok {
		defer closer.Close()
	}
	str, err := ocflv1.GetStore(ctx, fsys, root)
	if err != nil {
		log.Error(err, "could not read storage root", "path", root)
		return
	}
	if validateFlags.objectID != "" {
		pth, err := str.ResolveID(validateFlags.objectID)
		if err != nil {
			log.Error(err, "invalid object ID")
			return
		}
		name := path.Join(root, pth)
		_, result := ocflv1.ValidateObject(ctx, fsys, name, ocflv1.ValidationLogger(log))

		if err := result.Err(); err != nil {
			log.Error(err, "object is not valid")
			return
		}
		log.Info("object is valid", "id", validateFlags.objectID)
		return
	}
	result := str.Validate(ctx, ocflv1.ValidationLogger(log))

	if err := result.Err(); err != nil {
		log.Error(err, "store is not valid")
		return
	}
	log.Info("store is valid")
}
