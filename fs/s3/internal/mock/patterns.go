package mock

import (
	"fmt"
	"math/rand/v2"
)

func RandBytes(size int64) []byte {
	bytes := make([]byte, size)
	for i := range size {
		bytes[i] = byte(rand.Uint())
	}
	return bytes
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
	for i := range numDirs {
		objects[i].Key = fmt.Sprintf("%s-dir-%d/tmp.txt", prefix, i)
		objects[i].ContentLength = 1
		ret[i] = &objects[i]
	}
	for i := range numFiles {
		offset := i + numDirs
		objects[offset].Key = fmt.Sprintf("%s-file-%d.txt", prefix, i)
		objects[offset].ContentLength = 1
		ret[offset] = &objects[offset]
	}
	return ret
}
