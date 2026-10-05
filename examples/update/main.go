package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/fs/config"
	"github.com/srerickson/ocfl-go/fs/local"
)

type cmdFlags struct {
	objPath   string // path to object
	srcDir    string // path to content directory
	stageFile string // path to stage file
	msg       string // message for new version
	algID     string // digest algorith (sha512 or sha256)
	newID     string // ID for new object
	user      ocfl.User
}

// checkNewVersion returns an error if flags needed for a new version are
// missing.
func (f *cmdFlags) checkNewVersion() error {
	var missing []string
	for name, value := range map[string]string{
		"src":   f.srcDir,
		"msg":   f.msg,
		"name":  f.user.Name,
		"email": f.user.Address,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return errors.New("missing required flags: " + strings.Join(missing, ", "))
	}
	return nil
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stderr); err != nil {
		os.Exit(1)
	}
}

// run runs the command with args, reporting errors to stderr.
func run(ctx context.Context, args []string, stderr io.Writer) error {
	err := runUpdate(ctx, args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
	}
	return err
}

func runUpdate(ctx context.Context, args []string, stderr io.Writer) error {
	f, err := parseArgs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	objCnf, err := config.New(ctx, f.objPath, config.WithLogger(logger))
	if err != nil {
		return err
	}
	// an existing stage file is an update that was interrupted: resume it.
	stage, err := loadStage(f.stageFile)
	if err != nil {
		return err
	}
	if stage != nil {
		logger.Info("resuming update", "stage", f.stageFile)
	} else {
		if stage, err = newStage(ctx, objCnf, f); err != nil {
			return err
		}
		if err := saveStage(f.stageFile, stage); err != nil {
			return err
		}
	}
	if err := stage.Content.Open(ctx, config.Registry(config.WithLogger(logger))); err != nil {
		return err
	}
	applyCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	_, err = stage.Update.Apply(applyCtx, objCnf.FS, objCnf.Path, stage.Content, ocfl.UpdateWithLogger(logger))
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			// the stage file is kept, so the update can be resumed
			return err
		}
		logger.Info("received interupt: reverting changes...")
		if err := stage.Update.Revert(ctx, objCnf.FS, objCnf.Path, ocfl.UpdateWithLogger(logger)); err != nil {
			return err
		}
	}
	if f.stageFile != "" {
		return os.Remove(f.stageFile)
	}
	return nil
}

// newStage returns a finalized stage for a new version of the object with
// the contents of the source directory.
func newStage(ctx context.Context, objCnf *config.FSConfig, f *cmdFlags) (*ocfl.Stage, error) {
	if err := f.checkNewVersion(); err != nil {
		return nil, err
	}
	alg, err := digest.DefaultRegistry().Get(f.algID)
	if err != nil {
		return nil, err
	}
	update, err := ocfl.NewUpdate(ctx, objCnf.FS, objCnf.Path, f.newID, ocfl.UpdateWithDigestAlgorithm(alg))
	if err != nil {
		if errors.Is(err, ocfl.ErrNoObjectID) {
			return nil, errors.New("'id' flag is required for to a create new objects (object does not exist)")
		}
		return nil, fmt.Errorf("%s: %w", objCnf.Path, err)
	}
	// the new version state is the contents of srcDir
	if err := update.Clear(); err != nil {
		return nil, err
	}
	srcFS, err := local.NewFS(f.srcDir)
	if err != nil {
		return nil, err
	}
	stage := ocfl.NewStage(update)
	if err := stage.AddFS(ctx, srcFS, ".", "."); err != nil {
		return nil, err
	}
	if err := update.Finalize(f.msg, f.user); err != nil {
		return nil, err
	}
	return stage, nil
}

// loadStage loads the stage saved in the file name. It returns nil and no
// error if name is empty or the file doesn't exist.
func loadStage(name string) (*ocfl.Stage, error) {
	if name == "" {
		return nil, nil
	}
	data, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stage ocfl.Stage
	if err := json.Unmarshal(data, &stage); err != nil {
		return nil, fmt.Errorf("loading stage from %s: %w", name, err)
	}
	return &stage, nil
}

// saveStage saves the stage in the file name, if name isn't empty.
func saveStage(name string, stage *ocfl.Stage) error {
	if name == "" {
		return nil
	}
	data, err := json.Marshal(stage)
	if err != nil {
		return err
	}
	return os.WriteFile(name, data, 0o644)
}

func parseArgs(args []string, stderr io.Writer) (*cmdFlags, error) {
	var f cmdFlags
	set := flag.NewFlagSet("update", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.StringVar(&f.objPath, "obj", "", "path to ocfl object to create/update (required)")
	set.StringVar(&f.srcDir, "src", "", "local path with new object content")
	set.StringVar(&f.stageFile, "stage", "", "file for saving the update before applying it. If it exists, the update in it is resumed.")
	set.StringVar(&f.msg, "msg", "", "message field for new version")
	set.StringVar(&f.user.Name, "name", "", "name field for new version")
	set.StringVar(&f.user.Address, "email", "", "email field for new version")
	set.StringVar(&f.algID, "alg", "sha512", "digest algorithm for a new object (ignored for existing objects)")
	set.StringVar(&f.newID, "id", "", "object ID (required for creating new objects)")
	if err := set.Parse(args); err != nil {
		return nil, err
	}
	if f.objPath == "" {
		return nil, errors.New("missing required flag: obj")
	}
	return &f, nil
}
