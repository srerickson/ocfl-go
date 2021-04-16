# OCFL

[![](https://godoc.org/github.com/srerickson/ocfl?status.svg)](https://godoc.org/github.com/srerickson/ocfl)
[![Go Report Card](https://goreportcard.com/badge/github.com/srerickson/ocfl)](https://goreportcard.com/report/github.com/srerickson/ocfl)
[![Build Status](https://travis-ci.org/srerickson/ocfl.svg?branch=master)](https://travis-ci.org/srerickson/ocfl)

This is a Go module for working with [OCFL](https://ocfl.io/) objects. Some notable features:

- File system access is abstracted using the `io/fs.FS` interface (Go v1.16+). OCFL objects can be read from any backend supporting the `fs.FS` interface.
- Similarly, the logical content of an OCFL object is presented as an `fs.FS` (see example below).
- Object validation (*forthcoming*)
- Object creation & Object commits (*forthcoming*)

# Example Usage

```go
	root := os.DirFS(`object-root`)
	obj, err := ocfl.NewObjectReader(root)
	if err != nil {
		log.Fatal(err)
	}
	// obj is an fs.FS
	file, err := obj.Open(`v1/foo/bar.xml`)
	if err != nil {
		log.Fatal(err)
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	expected := "Me, Myself, I"
	if !strings.Contains(string(data), expected) {
		log.Fatalf("expected file to contain %s", expected)
	}
```


