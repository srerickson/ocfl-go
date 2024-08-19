package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/aws/aws-sdk-go-v2/config"
	awsS3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/local"
	"github.com/srerickson/ocfl-go/backend/s3"
	"github.com/srerickson/ocfl-go/logging"
)

var cli struct {
	InitRoot InitRootCmd `cmd:"init-root" help:"Initialize a new storage root"`
	Commit   CommitCmd   `cmd:"commit" help:"Create or update an object in a storage root"`
}

func main() {
	ctx := context.Background()
	kongCtx := kong.Parse(&cli,
		kong.Name("ocfl"),
		kong.Description("command line tool for working with OCFL repositories"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}))
	var err error
	switch kongCtx.Command() {
	case "init-root <path>":
		err = cli.InitRoot.Run(ctx, os.Stdout, os.Stderr)
	case "commit <path>":
		err = cli.Commit.Run(ctx, os.Stdout, os.Stderr)
	default:
		kongCtx.FatalIfErrorf(errors.New("invalid sub-command"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func parseRootConfig(ctx context.Context, name string) (ocfl.WriteFS, string, error) {
	//if we were using s3-based backend:
	rl, err := url.Parse(name)
	if err != nil {
		return nil, "", err
	}
	switch rl.Scheme {
	case "s3":
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, "", err
		}
		fsys := &s3.BucketFS{
			S3:     awsS3.NewFromConfig(cfg),
			Bucket: rl.Host,
			Logger: logging.DefaultLogger(),
		}
		return fsys, strings.TrimPrefix(rl.Path, "/"), nil
	default:
		absPath, err := filepath.Abs(name)
		if err != nil {
			return nil, "", err
		}
		fsys, err := local.NewFS(absPath)
		if err != nil {
			return nil, "", err
		}
		return fsys, ".", nil
	}
}

func rootConfig(fsys ocfl.WriteFS, dir string) string {
	switch fsys := fsys.(type) {
	case *s3.BucketFS:
		return "s3://" + path.Join(fsys.Bucket, dir)
	case *local.FS:
		return fsys.Root()
	default:
		panic(errors.New("unsupported backend type"))
	}
}
