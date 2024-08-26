package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	LS       LSCmd       `cmd:"ls" help:"List objects in a storage root or contents of an object version"`
	Export   ExportCmd   `cmd:"export" help:"Export object contents to the local filesystem"`
}

type runner interface {
	Run(ctx context.Context, stdout, stderr io.Writer) error
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
	var r runner
	switch kongCtx.Command() {
	case "init-root <path>":
		r = &cli.InitRoot
	case "commit <path>":
		r = &cli.Commit
	case "ls":
		r = &cli.LS
	case "export <dst>":
		r = &cli.Export
	default:
		kongCtx.FatalIfErrorf(errors.New("invalid sub-command"))
		return
	}
	if err := r.Run(ctx, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func parseRootConfig(ctx context.Context, name string) (ocfl.WriteFS, string, error) {
	if name == "" {
		return nil, "", fmt.Errorf("the storage root to use for the operation is not set")
	}
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
