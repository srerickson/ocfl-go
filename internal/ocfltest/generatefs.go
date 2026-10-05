package ocfltest

import (
	"math/rand"
	"path"
	"sort"
)

func GenerateFS(genr *rand.Rand, numFiles, maxSize int) SeedFS {
	fsys := SeedFS{}
	for i := 0; i < numFiles; i++ {
		size := genr.Intn(maxSize-1) + 1
		seed := genr.Int63()
		dir := randomDir(genr, fsys)
		// newdir
		if genr.Float32() < 0.15 {
			var newdir string
			for {
				newdir = path.Join(dir, randomName(genr, 12))
				if !exists(fsys, newdir) {
					break
				}
			}
			dir = newdir
		}
		var name string
		for {
			name = path.Join(dir, randomName(genr, 12))
			if !exists(fsys, name) {
				break
			}
		}
		fsys[name] = &SeedFile{Seed: seed, Size: int64(size)}
	}
	return fsys
}

func allDirs(fsys SeedFS) []string {
	var dummy struct{}
	dirs := map[string]struct{}{".": dummy}
	for name := range fsys {
		for _, p := range allParents(name) {
			dirs[p] = dummy
		}
	}
	dirslice := make([]string, len(dirs))
	i := 0
	for d := range dirs {
		dirslice[i] = d
		i++
	}
	sort.Strings(dirslice)
	return dirslice
}

func allParents(name string) []string {
	var parents []string
	for {
		dir := path.Dir(name)
		parents = append(parents, dir)
		if dir == "." {
			break
		}
		name = dir
	}
	return parents
}

func exists(fsys SeedFS, name string) bool {
	if _, exists := fsys[name]; exists {
		return true
	}
	for n := range fsys {
		for _, p := range allParents(n) {
			if p == name {
				return true
			}
		}
	}
	return false
}

func randomDir(genr *rand.Rand, fsys SeedFS) string {
	dirs := allDirs(fsys)
	return dirs[genr.Intn(len(dirs))]
}

func randomName(genr *rand.Rand, l int) string {
	var letters = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_ -()?")
	size := genr.Intn(l) + 1
	part := make([]rune, size)
	for j := range part {
		part[j] = letters[genr.Intn(len(letters))]
	}
	return string(part)
}
