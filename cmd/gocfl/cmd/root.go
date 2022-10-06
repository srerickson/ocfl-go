package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/iand/logfmtr"
	"github.com/muesli/coral"
)

const defaultCfg = `.gocfl.yaml`

var (
	// cfgFile is complete path to configuation file
	rootFlags = struct {
		cfgFile      string
		repoName     string
		driver       string // override repo settings
		driverPath   string // override repo settings
		driverBucket string // override repo settings
		saveConfig   bool
		verbose      bool
	}{}

	// rootCmd represents the base command when called without any subcommands
	rootCmd = &coral.Command{
		Use:          "gocfl",
		Short:        "A command line tool for OCFL",
		Long:         "A command line tool for working with OCFL Storage Roots and Objects.",
		SilenceUsage: true,
	}

	log = logfmtr.NewWithOptions(logfmtr.Options{
		Writer:    os.Stderr,
		Colorize:  true,
		Humanize:  true,
		NameDelim: "/",
	})
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx := context.Background()
	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		//log.Error(err, "quiting")
		os.Exit(1)
	}
}

func init() {
	coral.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&rootFlags.cfgFile, "config", "c", "", "config file (default is HOME/.gocfl.yaml)")
	rootCmd.PersistentFlags().StringVarP(&rootFlags.repoName, "repo", "r", "default", "name of repo in configuration to use")
	rootCmd.PersistentFlags().StringVarP(&rootFlags.driver, "driver", "d", "", "override active repo's 'driver' setting")
	rootCmd.PersistentFlags().StringVarP(&rootFlags.driverPath, "path", "p", "", "override active repo's 'path' setting")
	rootCmd.PersistentFlags().StringVarP(&rootFlags.driverBucket, "bucket", "b", "", "override active repo's 'bucket' setting")
	rootCmd.PersistentFlags().BoolVarP(&rootFlags.verbose, "verbose", "v", false, "override active repo's 'bucket' setting")
}

func initConfig() {
	if rootFlags.verbose {
		logfmtr.SetVerbosity(10)
	}

	if rootFlags.cfgFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Error(err, "can't determine home directory")
		}
		rootFlags.cfgFile = filepath.Join(home, defaultCfg)
	}
}
