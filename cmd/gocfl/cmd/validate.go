package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/muesli/coral"
	"github.com/srerickson/ocfl/ocflv1"
	"github.com/srerickson/ocfl/validation"
)

var validateFlags = struct {
	objPath string
}{}

// validateCmd represents the validate command
var validateCmd = &coral.Command{
	Use:   "validate",
	Short: "Validates an OCFL Object or Storage Root",
	Long:  `Validates an OCFL Object or Storage Root`,
	RunE:  runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().StringVarP(&validateFlags.objPath, "object", "o", "", "path to object")
}

func runValidate(cmd *coral.Command, args []string) error {
	if validateFlags.objPath != "" {
		return validateFSObject(cmd.Context(), validateFlags.objPath)
	}
	cfg, err := getConfig(cfgFile)
	if err != nil {
		return err
	}
	bk, root, err := cfg.getBackendPath(repoName)
	if err != nil {
		return err
	}
	vCfg := ocflv1.ValidateStoreConf{Log: validation.NewLog(log)}
	err = ocflv1.ValidateStore(cmd.Context(), bk, root, &vCfg)
	if err != nil {
		return err
	}
	return nil
}

func validateFSObject(ctx context.Context, name string) error {
	name, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	dir := filepath.Dir(name)
	base := filepath.Base(name)
	fsys := os.DirFS(dir)
	vCfg := ocflv1.ValidateObjectConf{
		Log: validation.NewLog(log),
	}
	err = ocflv1.ValidateObject(ctx, fsys, base, &vCfg)
	if err != nil {
		log.Error(err, "not a valid object")
	}
	return nil
}
