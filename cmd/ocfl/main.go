package main

import (
	"context"
	"os"

	"github.com/srerickson/ocfl-go/cmd/ocfl/run"
)

func main() {
	ctx := context.Background()
	if err := run.CLI(ctx, os.Args, os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}
