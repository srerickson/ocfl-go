package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/goccy/go-yaml"
	"github.com/muesli/coral"
	"github.com/srerickson/ocfl"
)

const (
	defaultRepoName = "default"
	s3RepoType      = "s3"
	fsRepoType      = "fs"
)

type Config struct {
	Name  string                `yaml:"name"`
	Email string                `yaml:"email"`
	Repos map[string]RepoConfig `yaml:"repos"`
}

type RepoConfig struct {
	Type string `yaml:"type"` // repo type: "fs" or "s3"

	// store_path is the location of an OCFL Storage Root. It is evaluatated in
	// the context of a backend configuration (see below). Slash-separated
	// sequences of path elements, like “x/y/z” are supported. "/" should always
	// be used as a path separator. Values must not contain an element that is
	// “.” or “..” or the empty string. Paths must not start or end with a
	// slash: “/x” and “x/” are invalid.
	StorePath string `yaml:"store_path"`

	// fs_root is used for the fs backend configuration: it should be an
	// absolute path to a directory where OCFL storage roots are created (the
	// parent directory for an OCFL storage root, not the storage root itself).
	// The default value is the current working directory. The store_path
	// setting is interpreted as a relative path (using "/" as a path separator)
	// to a director under fs_root.
	Root    *string `yaml:"fs_root,omitempty"`
	absRoot string  // absolute path version of Root

	// S3 backend configutation. The store_path setting is interpreted as
	// the prefix for objects in the storage root.
	Endpoint *string `yaml:"s3_endpoint,omitempty"`
	Bucket   *string `yaml:"s3_bucket,omitempty"`
	Region   *string `yaml:"s3_region,omitempty"`
}

// configCmd represents the config command
var configCmd = &coral.Command{
	Use:   "config",
	Short: "print configs",
	Long:  "print gocfl configuration",
	RunE:  runConfig,
	Args:  coral.MaximumNArgs(2),
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *coral.Command, args []string) error {
	conf, err := getConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return printConfig(conf)
}

func getConfig(name string) (*Config, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", name, err)
	}
	defer f.Close()
	var cfg Config
	err = yaml.NewDecoder(f).Decode(&cfg)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("config error: %w", err)
	} else if err != nil {
		log.Info("using default config, not found: %s", name)
		return &Config{}, nil
	}
	log.WithValues("file", name).Info("read config")
	return &cfg, nil
}

func (cfg Config) getRepoConfig(name string) (RepoConfig, error) {
	if name == "" {
		name = defaultRepoName
	}
	repo, ok := cfg.Repos[name]
	if !ok {
		return RepoConfig{}, fmt.Errorf("repo not configured: %s", name)
	}
	return repo, nil
}

func (cfg Config) getBackendPath(name string) (ocfl.FS, string, error) {
	return nil, "", errors.New("FIXME")

	// repo, err := cfg.getRepoConfig(name)
	// if err != nil {
	// 	return nil, "", err
	// }
	// if bucket, awsCfg := repo.awsConfig(); bucket != "" {
	// 	sess, err := session.NewSession(awsCfg)
	// 	if err != nil {
	// 		return nil, "", fmt.Errorf("backend config: %w", err)
	// 	}
	// 	vals := []any{
	// 		"repo_type", s3RepoType,
	// 		"bucket", bucket,
	// 	}
	// 	log.Info("backend settings", vals...)
	// 	return s3fs.New(s3.New(sess), bucket), repo.StorePath, nil
	// }
	// if root := repo.fsConfig(); root != "" {
	// 	bak, err := local.NewBackend(root)
	// 	if err != nil {
	// 		return nil, "", fmt.Errorf("backend config: %w", err)
	// 	}
	// 	vals := []any{
	// 		"type", fsRepoType,
	// 		"root", root,
	// 	}
	// 	log.Info("backend settings", vals...)
	// 	return bak, repo.StorePath, nil
	// }
	// return nil, "", fmt.Errorf("could not determine repo type")

}

// func writeConfig(name string, cfg *Config) error {
// 	f, err := os.OpenFile(name, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
// 	if err != nil {
// 		return fmt.Errorf("saving config %s: %w", name, err)
// 	}
// 	defer f.Close()
// 	err = yaml.NewEncoder(f).Encode(cfg)
// 	if err != nil {
// 		return fmt.Errorf("saving config %s: %w", name, err)
// 	}
// 	log.WithValues("file", name).Info("write config")
// 	return nil
// }

func printConfig(cfg *Config) error {
	return yaml.NewEncoder(os.Stdout).Encode(cfg)
}

// return s3 bucket and aws config from config
func (repo *RepoConfig) awsConfig() (string, *aws.Config) {
	if repo.Type != s3RepoType || repo.Bucket == nil {
		return "", nil
	}
	awsCfg := aws.Config{
		Region:   repo.Region,
		Endpoint: repo.Endpoint,
	}
	return *repo.Bucket, &awsCfg
}

// return root
func (repo *RepoConfig) fsConfig() string {
	if repo.Type != fsRepoType {
		return ""
	}
	if repo.Root == nil {
		return ""
	}
	root := filepath.Clean(*repo.Root)
	if !filepath.IsAbs(root) {
		wd, err := os.Getwd()
		if err != nil {
			log.Error(err, "error during backend configuration")
			return ""
		}
		root = filepath.Join(wd, root)
	}
	repo.absRoot = root
	return root
}
