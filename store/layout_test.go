package store_test

import (
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl/store"
)

func TestLayouts(t *testing.T) {
	var memfs = fstest.MapFS{
		"store-1/extensions/0002-flat-direct-storage-layout/config.json": &fstest.MapFile{
			Data: []byte(`{"extensionName": "0002-flat-direct-storage-layout"}`),
		},
		"store-2/extensions/0004-hashed-n-tuple-storage-layout/.keeo": &fstest.MapFile{
			Data: []byte(``),
		},
		"store-3/extensions/0004-hashed-n-tuple-storage-layout/config.json": &fstest.MapFile{
			Data: []byte(`{
				"extensionName": "0004-hashed-n-tuple-storage-layout",
				"shortObjectRoot": true,
				"tupleSize": 4,
				"numberOfTuples": 2
				}`),
		},
	}

	// 0002-flat-direct-storage-layout
	layout, err := store.ReadLayoutFunc(memfs, "store-1", "0002-flat-direct-storage-layout")
	if err != nil {
		t.Fatal(err)
	}
	id := "this/is/my/id"
	p, err := layout(id)
	if err != nil {
		t.Fatal(err)
	}
	if p != id {
		t.Errorf("expected layour to return %s, got %s", id, p)
	}

	// 0004-hashed-n-tuple-storage-layout
	layout, err = store.ReadLayoutFunc(memfs, "store-2", "0004-hashed-n-tuple-storage-layout")
	if err != nil {
		t.Fatal(err)
	}
	id = "object-01"
	expected := "3c0/ff4/240/3c0ff4240c1e116dba14c7627f2319b58aa3d77606d0d90dfc6161608ac987d4"
	p, err = layout(id)
	if err != nil {
		t.Fatal(err)
	}
	if p != expected {
		t.Errorf("expected layour to return %s, got %s", expected, p)
	}

	// 0004-hashed-n-tuple-storage-layout
	layout, err = store.ReadLayoutFunc(memfs, "store-3", "0004-hashed-n-tuple-storage-layout")
	if err != nil {
		t.Fatal(err)
	}
	id = "object-01"
	expected = "3c0f/f424/0c1e116dba14c7627f2319b58aa3d77606d0d90dfc6161608ac987d4"
	p, err = layout(id)
	if err != nil {
		t.Fatal(err)
	}
	if p != expected {
		t.Errorf("expected layour to return %s, got %s", expected, p)
	}

}
