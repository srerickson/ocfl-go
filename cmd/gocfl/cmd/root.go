package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/iand/logfmtr"
	"github.com/muesli/coral"
)

const (
	defaultCfg = `.gocfl.yaml`
)

var (
	// cfgFile is complete path to configuation file
	cfgFile  string
	repoName string

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

	// trap Ctrl+C and call cancel on the context
	// ctx, cancel := context.WithCancel(ctx)
	// c := make(chan os.Signal, 1)
	// signal.Notify(c, os.Interrupt)
	// defer func() {
	// 	signal.Stop(c)
	// 	cancel()
	// }()
	// go func() {
	// 	select {
	// 	case <-c:
	// 		cancel()
	// 	case <-ctx.Done():
	// 	}
	// }()
	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	coral.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.gocfl.yaml)")
	rootCmd.PersistentFlags().StringVarP(&repoName, "repo", "r", "", "name of repo in configuration to use")

}

func initConfig() {
	if cfgFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Error(err, "could not get home dir")
		}
		cfgFile = filepath.Join(home, defaultCfg)
	}
}
