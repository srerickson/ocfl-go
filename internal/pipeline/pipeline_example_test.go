package pipeline_test

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"

	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/internal/pipeline"
)

var alg = `sha256`

func ExampleResults() {
	fsys := os.DirFS(".")
	type job struct {
		name string
	}
	type result struct {
		name string
		sum  string
	}
	var walkErr error
	inputFn := func(add func(job) bool) {
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
		dig := digest.NewMultiDigester(alg)
		if _, err := io.Copy(dig, f); err != nil {
			return r, err
		}
		r.sum = dig.Sums()[alg]
		return r, nil
	}
	pipeline.Results(inputFn, workFn, 0)(func(r pipeline.Result[job, result]) bool {
		if r.Out.name == "pipeline.go" && r.Out.sum != "" {
			fmt.Println(r.Out.name)
			// Output: pipeline.go
		}
		return true
	})
	if walkErr != nil {
		log.Fatal(walkErr)
	}
}
