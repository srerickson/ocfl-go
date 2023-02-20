package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/cloud"
	"github.com/srerickson/ocfl/ocflv1"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
)

var (
	bucket string
	dir    string
)

func main() {
	flag.StringVar(&bucket, "b", "", "bucket")
	flag.StringVar(&dir, "d", ".", "storage root directory")
	flag.Parse()
	ctx := context.Background()
	az, err := blob.OpenBucket(ctx, bucket)
	if err != nil {
		log.Fatal(err)
	}
	defer az.Close()
	timeScan(ctx, cloud.NewFS(az), dir, "object scan", ocflv1.ScanObjects)
}

func timeScan(ctx context.Context, fsys ocfl.FS, dir string, name string, scn func(context.Context, ocfl.FS, string, func(*ocflv1.Object) error, *ocflv1.ScanObjectsOpts) error) {
	opts := &ocflv1.ScanObjectsOpts{
		Concurrency: 100,
	}
	startScan := time.Now()
	numObjects := 0
	defer func() {
		log.Printf("%s: found %d objects %.03f seconds", name, numObjects, time.Since(startScan).Seconds())
	}()
	scanFn := func(obj *ocflv1.Object) error {
		numObjects++
		return nil
	}
	if err := scn(ctx, fsys, dir, scanFn, opts); err != nil {
		log.Println(err)
	}
}
