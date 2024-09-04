package run

import (
	"context"
	"fmt"
	"io"

	"github.com/srerickson/ocfl-go"
)

const commitHelp = "Create or update an object in a storage root"

type commitCmd struct {
	ID      string `name:"id" short:"i" help:"The ID for the object to create or update"`
	Message string `name:"message" short:"m" help:"Message to include in the object version metadata"`
	Name    string `name:"name" short:"n" env:"OCFL_USER_NAME" help:"Username to include in the object version metadata"`
	Email   string `name:"email" short:"e" env:"OCFL_USER_EMAIL" help:"User email to include in the object version metadata"`
	Spec    string `name:"ocflv" default:"1.1" help:"OCFL spec fo the new object"`
	Alg     string `name:"alg" default:"sha512" help:"Digest Algorithm used to digest content"`
	Path    string `arg:"" name:"path" help:"local directory with object state to commit"`
}

func (cmd *commitCmd) Run(ctx context.Context, root *ocfl.Root, stdout, stderr io.Writer) error {
	readFS := ocfl.DirFS(cmd.Path)
	obj, err := root.NewObject(ctx, cmd.ID)
	if err != nil {
		return err
	}
	stage, err := ocfl.StageDir(ctx, readFS, ".", cmd.Alg)
	if err != nil {
		return err
	}
	for name := range stage.State.PathMap() {
		fmt.Fprintln(stdout, name)
	}
	return obj.Commit(ctx, &ocfl.Commit{
		ID:      cmd.ID,
		Stage:   stage,
		Message: cmd.Message,
		User:    ocfl.User{Name: cmd.Name, Address: cmd.Email},
		Spec:    ocfl.Spec(cmd.Spec),
	})
}
