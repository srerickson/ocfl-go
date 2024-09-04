package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
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
	"github.com/srerickson/ocfl-go/ocflv1"
)

var cli struct {
	RootConfig string      `name:"root" short:"r" env:"OCFL_ROOT" help:"The prefix/directory of the OCFL storage root used for the command"`
	InitRoot   initRootCmd `cmd:"init-root" help:"${init_root_help}"`
	Commit     commitCmd   `cmd:"commit" help:"${commit_help}"`
	LS         lsCmd       `cmd:"ls" help:"${ls_help}"`
	Export     exportCmd   `cmd:"export" help:"${export_help}"`
	Diff       DiffCmd     `cmd:"diff" help:"${diff_help}"`
}

func CLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	ocflv1.Enable() // hopefuly this won't be necessary in the near future.
	parser, err := kong.New(&cli, kong.Name("ocfl"),
		kong.Writers(stdout, stderr),
		kong.Description("tools for working with OCFL repositories"),
		kong.Vars{
			"commit_help":    commitHelp,
			"diff_help":      diffHelp,
			"export_help":    exportHelp,
			"init_root_help": initRootHelp,
			"ls_help":        lsHelp,
		},
		kong.ConfigureHelp(kong.HelpOptions{
			Summary: true,
			Compact: true,
		}),
	)
	if err != nil {
		fmt.Fprintln(stderr, "in kong configuration:", err.Error())
		return err
	}
	kongCtx, err := parser.Parse(args[1:])
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		var parseErr *kong.ParseError
		if errors.As(err, &parseErr) {
			parseErr.Context.PrintUsage(true)
		}
		return err
	}
	// run a command on a non-existing root
	if kongCtx.Command() == "init-root" {
		if err := cli.InitRoot.Run(ctx, cli.RootConfig, stdout, stderr); err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		return nil
	}
	// run a command on existing root
	var runner interface {
		Run(ctx context.Context, root *ocfl.Root, stdout, stderr io.Writer) error
	}
	switch kongCtx.Command() {
	case "commit <path>":
		runner = &cli.Commit
	case "ls":
		runner = &cli.LS
	case "export":
		runner = &cli.Export
	case "diff":
		runner = &cli.Diff
	default:
		kongCtx.PrintUsage(true)
		err = fmt.Errorf("unknown command: %s", kongCtx.Command())
		fmt.Fprintln(stderr, err.Error())
		return err
	}
	fsys, dir, err := parseRootConfig(ctx, cli.RootConfig)
	if err != nil {
		fmt.Fprintln(stderr, "error in OCFL root configuration:", err.Error())
		return err
	}
	root, err := ocfl.NewRoot(ctx, fsys, dir)
	if err != nil {
		rootcnf := rootConfig(fsys, dir)
		fmt.Fprintln(stderr, "error reading OCFL storage root:", rootcnf+":", err.Error())
		return err
	}
	if err := runner.Run(ctx, root, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return err
	}
	return nil
}

func parseRootConfig(ctx context.Context, name string) (ocfl.WriteFS, string, error) {
	if name == "" {
		return nil, "", fmt.Errorf("the storage root location was not given")
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
