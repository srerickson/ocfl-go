package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/goccy/go-yaml"
	"github.com/muesli/coral"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/cloud"
	"github.com/srerickson/ocfl/backend/local"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	"gocloud.dev/blob/s3blob"
)

const (
	defaultRepoName = "default"
	fileDriver      = "file"
	s3Driver        = "s3"
	azureDriver     = "azure"
)

var configFlags = struct {
	saveConfig bool
}{}

type Config struct {
	Name  string                 `yaml:"name"`
	Email string                 `yaml:"email"`
	Repos map[string]*RepoConfig `yaml:"repos"`
}

type RepoConfig struct {
	Driver   string  `yaml:"driver"` // storage driver: "file", "s3", or "azure"
	Path     string  `yaml:"path,omitempty"`
	Bucket   *string `yaml:"bucket,omitempty"`
	Endpoint *string `yaml:"endpoint,omitempty"`
	Region   *string `yaml:"region,omitempty"`
}

// configCmd represents the config command
var configCmd = &coral.Command{
	Use:   "config",
	Short: "print configs",
	Long:  "print gocfl configuration",
	Run:   runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.Flags().BoolVar(&configFlags.saveConfig, "save", false, "save config used in current command")
}

func runConfig(cmd *coral.Command, args []string) {
	conf, err := getConfig()
	if err != nil {
		log.Error(err, "can't load config", "file", rootFlags.cfgFile)
		return
	}
	writer := io.Writer(os.Stdout)
	if configFlags.saveConfig {
		f, err := os.OpenFile(rootFlags.cfgFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			log.Error(err, "can't open config file for writing")
			return
		}
		defer f.Close()
		writer = io.MultiWriter(os.Stdout, f)
		log.Info("saving config to file", "file", rootFlags.cfgFile)
	}
	if err := yaml.NewEncoder(writer).Encode(conf); err != nil {
		log.Error(err, "error encoding or writing config")
	}
}

func getConfig() (*Config, error) {
	var cfg *Config
	name := rootFlags.cfgFile
	f, err := os.Open(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed read config file %s: %w", name, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		log.Info("config file not found", "file", name)
		log.Info("using default settings")
		cfg = &Config{
			Repos: map[string]*RepoConfig{
				defaultRepoName: defaultRepo(),
			},
		}
	}
	if f != nil {
		defer f.Close()
		cfg = &Config{}
		err = yaml.NewDecoder(f).Decode(cfg)
		if err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
		log.Info("read config", "file", name)
	}
	// apply root command flags
	repo := cfg.Repo(rootFlags.repoName, true)
	repo.applyRootFlags()
	return cfg, nil
}

func defaultRepo() *RepoConfig {
	return &RepoConfig{
		Driver: "file",
		Path:   ".",
	}
}

func (cfg *Config) Repo(name string, create bool) *RepoConfig {
	if name == "" {
		name = defaultCfg
	}
	repo := cfg.Repos[name]
	if repo == nil && create {
		repo = defaultRepo()
		cfg.Repos[name] = repo
	}
	return repo
}

func (cfg *Config) NewFSPath(ctx context.Context, name string) (ocfl.FS, string, error) {
	repo := cfg.Repo(name, false)
	if repo == nil {
		return nil, "", fmt.Errorf("no repo named '%s' in config", name)
	}
	return repo.GetFSPath(ctx)
}

func (repo *RepoConfig) GetFSPath(ctx context.Context) (ocfl.FS, string, error) {
	var (
		fsys ocfl.FS
		path string = repo.Path
		err  error
	)
	if path == "" {
		path = "."
	}
	switch repo.Driver {
	case fileDriver:
		// repo.Path is used to create the ocfl.FS, so the path returned is just "."
		// A problem with this is it means we can't open files outside the storage
		// root using this fsys.
		path = "."
		fsys, err = repo.NewLocalFS()
	case s3Driver:
		fsys, err = repo.NewS3FS(ctx) // fsys needs to be closed!
	case azureDriver:
		fsys, err = repo.NewAzureFS(ctx) // fsys needs to be closed!
	default:
		return nil, "", fmt.Errorf("invalid storage driver: '%s'", repo.Driver)
	}
	if err != nil {
		return nil, "", fmt.Errorf("in '%s' storage driver: %w", repo.Driver, err)
	}
	return fsys, path, nil
}

// return s3 bucket and aws config from config
func (repo *RepoConfig) NewS3FS(ctx context.Context) (*cloud.FS, error) {
	if repo.Bucket == nil {
		return nil, errors.New("'bucket' config is required")
	}
	bucketName := *repo.Bucket
	awsCfg := aws.Config{
		Region:   repo.Region,
		Endpoint: repo.Endpoint,
	}
	sess, err := session.NewSession(&awsCfg)
	if err != nil {
		return nil, err
	}
	bucket, err := s3blob.OpenBucket(ctx, sess, bucketName, nil)
	if err != nil {
		return nil, err
	}
	log.Info("storage backend settings", "driver", s3Driver, "bucket", bucketName)
	return cloud.NewFS(bucket, cloud.WithLogger(log)), nil
}

func (repo *RepoConfig) NewAzureFS(ctx context.Context) (*cloud.FS, error) {
	if repo.Bucket == nil {
		return nil, errors.New("'bucket' config is required")
	}
	bucketName := *repo.Bucket
	bucket, err := blob.OpenBucket(ctx, "azblob://"+bucketName)
	if err != nil {
		return nil, err
	}
	log.Info("storage backend settings", "driver", azureDriver, "container", bucketName)
	return cloud.NewFS(bucket, cloud.WithLogger(log)), nil
}

func (repo *RepoConfig) NewLocalFS() (*local.FS, error) {
	root := repo.Path
	if root == "" {
		root = "."
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(wd, root)
	}
	log.Info("storage backend settings", "driver", fileDriver, "root", root)
	return local.NewFS(root)
}

func (repo *RepoConfig) applyRootFlags() {
	if rootFlags.driver != "" {
		repo.Driver = rootFlags.driver
	}
	if rootFlags.driverPath != "" {
		repo.Path = rootFlags.driverPath
	}
	if rootFlags.driverBucket != "" {
		repo.Bucket = &rootFlags.driverBucket
	}
}
