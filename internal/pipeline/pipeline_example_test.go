package pipeline_test

import (
	"fmt"
	"io/fs"
	"log"
	"os"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/internal/pipeline"
)

func ExampleRun() {
	fsys := os.DirFS(".")
	type job struct {
		name string
	}
	type result struct {
		name string
		sum  string
	}
	var walkErr error
	setupFn := func(add func(job) bool) {
		walkErr = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if d.Type().IsRegular() {
				add(job{name: path})
			}
			return err
		})
	}
	workFn := func(j job) (result, error) {
		r := result{name: j.name}
		f, err := fsys.Open(j.name)
		if err != nil {
			return r, err
		}
		defer f.Close()
		dig := ocfl.NewMultiDigester(ocfl.SHA256)
		if _, err := dig.ReadFrom(f); err != nil {
			return r, err
		}
		r.sum = dig.Sums()[ocfl.SHA256]
		return r, nil
	}
	resultFn := func(in job, out result, err error) error {
		if out.name == "pipeline.go" && out.sum != "" {
			fmt.Println(out.name)
			// Output: pipeline.go
		}
		return nil
	}
	if err := pipeline.Run(setupFn, workFn, resultFn, 0); err != nil {
		log.Fatal(err)
	}
	if walkErr != nil {
		log.Fatal(walkErr)
	}
}
