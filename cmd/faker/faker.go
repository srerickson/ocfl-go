// faker is a program for building storage roots of arbitrary
// size with (small) random object content.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"runtime"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/cloud"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/internal/ocfltest"
	"github.com/srerickson/ocfl/internal/pipeline"
	"github.com/srerickson/ocfl/ocflv1"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
)

var (
	bucketName string
	storeDir   string
	numObjects int
	startNum   int
	seed       int64
	gos        int
)

const idPrefix = "fake-object-"

type objTask struct {
	id   string
	seed int64
}
type void struct{}

// const desc = "Faker generates small, fake OCFL objects in a specified storage root."

func main() {
	flag.StringVar(&bucketName, "bucket", "ocfl", "bucket name")
	flag.StringVar(&storeDir, "prefix", "faker", "storage root prefix")
	flag.IntVar(&numObjects, "num", 10, "number of fake objects to create")
	flag.IntVar(&startNum, "start", 1, "start index fake object")
	flag.Int64Var(&seed, "seed", 0, "random number generation seed")
	flag.IntVar(&gos, "gos", runtime.NumCPU(), "number of concurrent object creation go routines")
	flag.Parse()

	ctx := context.Background()
	bucket, err := blob.OpenBucket(ctx, "azblob://"+bucketName)
	if err != nil {
		log.Fatal(err)
	}
	fsys := cloud.NewFS(bucket)

	// load store, creating if necessary
	store, err := ocflv1.GetStore(ctx, fsys, storeDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Fatal(err)
	}
	if store == nil {
		log.Println("creating new store at", storeDir)
		layout := extensions.NewLayoutHashIDTuple()
		if err := ocflv1.InitStore(ctx, fsys, storeDir, &ocflv1.InitStoreConf{
			Spec:        ocfl.Spec{1, 1},
			Description: "fake layout",
			Layout:      layout,
		}); err != nil {
			log.Fatal(err)
		}
		store, err = ocflv1.GetStore(ctx, fsys, storeDir)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Println("store loaded", store.Description())
	log.Printf("creating %d objects [%s%d - %s%d]", numObjects, idPrefix, startNum, idPrefix, startNum+numObjects-1)

	setup := func(add func(objTask) error) error {
		for i := 0; i < numObjects; i++ {
			task := objTask{
				id:   fmt.Sprintf("%s%09d", idPrefix, i+startNum),
				seed: seed + int64(i),
			}
			if add(task); err != nil {
				break
			}
		}
		return nil
	}
	create := func(t objTask) (void, error) {
		genr := rand.New(rand.NewSource(t.seed))
		srcFS := ocfl.NewFS(ocfltest.GenerateFS(genr, 5, 1024))
		stage, _ := ocfl.NewStage(digest.SHA256(), digest.Map{}, srcFS)
		if err := stage.AddRoot(ctx, "."); err != nil {
			log.Fatal(err)
		}
		return void{}, store.Commit(ctx, t.id, stage)
	}
	results := func(t objTask, _ void, err error) error {
		if err != nil {
			return err
		}
		log.Println("created", t.id)
		return nil
	}
	if err := pipeline.Run(setup, create, results, gos); err != nil {
		log.Fatal(err)
	}
}
