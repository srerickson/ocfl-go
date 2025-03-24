package server

import (
	"iter"
	"maps"
	"slices"
	"time"

	"github.com/srerickson/ocfl-go"
)

type RootIndex interface {
	Objects() iter.Seq[*IndexObject]
	Get(id string) *IndexObject
	ReIndex(iter.Seq2[*ocfl.Object, error]) error
}

type IndexObject struct {
	Path        string
	ID          string
	Head        ocfl.VNum
	HeadCreated time.Time
}

type MapRootIndex struct {
	objects map[string]*IndexObject
}

func (m *MapRootIndex) Objects() iter.Seq[*IndexObject] {
	return func(yield func(*IndexObject) bool) {
		ids := slices.Collect(maps.Keys(m.objects))
		slices.Sort(ids)
		for _, id := range ids {
			if !yield(m.objects[id]) {
				return
			}
		}
	}
}

func (m *MapRootIndex) Get(id string) *IndexObject {
	if m.objects == nil {
		return nil
	}
	return m.objects[id]
}

func (m *MapRootIndex) ReIndex(objects iter.Seq2[*ocfl.Object, error]) error {
	m.objects = map[string]*IndexObject{}
	for obj := range objects {
		m.objects[obj.ID()] = &IndexObject{
			Path:        obj.Path(),
			ID:          obj.ID(),
			Head:        obj.Inventory().Head(),
			HeadCreated: obj.Inventory().Version(0).Created(),
		}
	}
	return nil
}
