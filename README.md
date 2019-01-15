# OCFL in Go

[![](https://godoc.org/github.com/srerickson/ocfl?status.svg)](https://godoc.org/github.com/srerickson/ocfl)
[![Go Report Card](https://goreportcard.com/badge/github.com/srerickson/ocfl)](https://goreportcard.com/report/github.com/srerickson/ocfl)

This is an implementation of [OCFL](https://ocfl.io/) in Go. *It is work-in-progress and **should not** be used IRL*. 

# Usage

## Create/Update an OCFL Object

```go
// Create a new empty object called example-1
object, _ := InitObject(`path/to/object-example-1`, `example-1-id`)

// Staging area for building new versions
stage, _ := object.NewStage()
file, _ := stage.OpenFile(filepath.Join(`dir`, `test-1.txt`))
file.WriteString(`...`)
file.Close()

// Something like this for importing contents of a directory:
stage, _ := object.NewStageDir(`path/to/data`)

// Create the new version
stage.Commit(NewUser(`somebody`, `some@where`), `commit version 1`)
```

## Validation

```go
if err := ValidateObject(`path/to/object-example-1`); err != nil {
    // not valid
}
```