package mock

import (
	"fmt"
	"io"
	"path"

	"golang.org/x/exp/rand"
)

func RandBytes(seed uint64, size int64) []byte {
	genr := rand.New(rand.NewSource(seed))
	buf, err := io.ReadAll(io.LimitReader(genr, size))
	if err != nil {
		panic(err)
	}
	return buf
}

func DirectoryList(numFiles, numDirs int, prefix string) []*Object {
	if prefix == "" {
		prefix = "tmp"
	}
	if numDirs < 0 || numFiles < 0 {
		return nil
	}
	objects := make([]Object, numFiles+numDirs)
	ret := make([]*Object, len(objects))
	for i := 0; i < numDirs; i++ {
		objects[i].Key = fmt.Sprintf("%s-dir-%d/tmp.txt", prefix, i)
		objects[i].ContentLength = 1
		ret[i] = &objects[i]
	}
	for i := 0; i < numFiles; i++ {
		offset := i + numDirs
		objects[offset].Key = fmt.Sprintf("%s-file-%d.txt", prefix, i)
		objects[offset].ContentLength = 1
		ret[offset] = &objects[offset]
	}
	return ret
}

func StorageRoot(seed uint64, prefix string, numObjects int) []*Object {
	objects := []*Object{
		{Key: path.Join(prefix, "0=ocfl_1.1")},
		{Key: path.Join(prefix, "extensions/mylayout/config.json")},
		{Key: path.Join(prefix, "ocfl_1.1.md")},
	}
	genr := rand.New(rand.NewSource(seed))
	for i := 0; i < numObjects; i++ {
		part := fmt.Sprintf("%s-%d", randPathPart(genr, 5, 8), i)
		newObjects := []*Object{
			{Key: path.Join(prefix, part, "0=ocfl_object_1.1")},
			{Key: path.Join(prefix, part, "inventory.json")},
			{Key: path.Join(prefix, part, "inventory.json.sha512")},
			{Key: path.Join(prefix, part, "v1/contents/file.txt")},
			{Key: path.Join(prefix, part, "extensions/ext01/config.json")},
		}
		objects = append(objects, newObjects...)
	}
	return objects
}

func randPathPart(genr *rand.Rand, minSize, maxSize int) string {
	const chars = `abcdefghijklmnopqrstuvwzyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890-_. `
	const lenChars = len(chars)
	size := minSize
	if size < 1 {
		size = 1
	}
	if maxSize > size {
		size += genr.Intn(maxSize - size + 1)
	}
	out := ""
	for i := 0; i < size; i++ {
		var next byte
		for {
			next = chars[genr.Intn(lenChars)]
			if next == '.' && i > 0 && out[i-1] == '.' {
				// dont allow '..'
				continue // try again
			}
			if size == 1 && next == '.' {
				continue
			}
			break
		}
		out += string(next)
	}
	return out
}
