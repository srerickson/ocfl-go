package main

import (
	"context"
	"fmt"
	"io"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
)

type CommitCmd struct {
	ID      string `name:"id" short:"i" help:"The ID for the object to create or update"`
	Root    string `name:"root" short:"r" env:"OCFL_ROOT" help:"The prefix/directory of the OCFL storage root for the object"`
	Message string `name:"message" short:"m" help:"Message to include in the object version metadata"`
	Name    string `name:"name" short:"n" env:"OCFL_USER_NAME" help:"Username to include in the object version metadata"`
	Email   string `name:"email" short:"e" env:"OCFL_USER_EMAIL" help:"User email to include in the object version metadata"`
	Spec    string `name:"ocflv" default:"1.1" help:"OCFL spec fo the new object"`
	Alg     string `name:"alg" default:"sha512" help:"Digest Algorithm used to digest content"`
	Path    string `arg:"" name:"path" help:"local directory with object state to commit"`
}

func (cmd *CommitCmd) Run(ctx context.Context, stdout, stderr io.Writer) error {
	ocflv1.Enable() // hopefuly this won't be necessary in the near future.
	readFS := ocfl.DirFS(cmd.Path)
	writeFS, dir, err := parseRootConfig(ctx, cmd.Root)
	if err != nil {
		return err
	}
	root, err := ocfl.NewRoot(ctx, writeFS, dir)
	if err != nil {
		return err
	}
	obj, err := root.NewObject(ctx, cmd.ID)
	if err != nil {
		return err
	}
	stage, err := ocfl.StageDir(ctx, readFS, ".", cmd.Alg)
	if err != nil {
		return err
	}
	for name := range stage.State.PathMap() {
		fmt.Println(name)
	}
	return obj.Commit(ctx, &ocfl.Commit{
		ID:      cmd.ID,
		Stage:   stage,
		Message: cmd.Message,
		User:    ocfl.User{Name: cmd.Name, Address: cmd.Email},
		Spec:    ocfl.Spec(cmd.Spec),
	})
}
