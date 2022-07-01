package cmd

import (
	"fmt"

	"github.com/muesli/coral"
	"github.com/srerickson/ocfl/ocflv1"
)

// statCmd represents the stat command
var statCmd = &coral.Command{
	Use:   "stat",
	Short: "Summary info on storage root or object",
	Long:  "Print useful information about an OCFL storage root or object",
	RunE:  runStat,
}

func init() {
	rootCmd.AddCommand(statCmd)
}

func runStat(cmd *coral.Command, args []string) error {
	conf, err := getConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	bk, root, err := conf.getBackendPath(repoName)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	str, err := ocflv1.GetStore(cmd.Context(), bk, root)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if desc := str.Description(); desc != "" {
		fmt.Println(desc)
	}
	scan, err := str.ScanObjects(cmd.Context())
	if err != nil {
		return fmt.Errorf("scanning storage root: %w", err)
	}
	for p := range scan {
		obj, err := str.GetPath(cmd.Context(), p)
		if err != nil {
			return fmt.Errorf("listing objects: %w", err)
		}
		id, err := obj.ID(cmd.Context())
		if err != nil {
			return fmt.Errorf("reading object: %w", err)
		}
		head, err := obj.Head(cmd.Context())
		if err != nil {
			return fmt.Errorf("reading %s: %w", id, err)
		}
		ver, err := obj.Version(cmd.Context(), head)
		if err != nil {
			return fmt.Errorf("reading object %s: %w", id, err)
		}
		fmt.Printf("%s %s [%v]\n", id, head.String(), ver.Created.Format("2006-01-02 15:04"))
	}
	return nil
}
