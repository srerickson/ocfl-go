# An OCFL implementation for Go


<a href="https://godoc.org/github.com/srerickson/ocfl">
    <img src="https://godoc.org/github.com/srerickson/ocfl?status.svg" alt="godocs"/>
</a>
<a href="https://goreportcard.com/report/github.com/srerickson/ocfl">
    <img src="https://goreportcard.com/badge/github.com/srerickson/ocfl">
</a>

This is an implementation of the Oxford Common File Layout
([OCFL](https://ocfl.io/) ) for Go. A command line interface (`gocfl`) for
creating, updating, and validating OCFL storage roots and objects is also
included. 

*Both the API and CLI are under heavy development and will have constant
breaking changes*!

## What is OCFL?

> This Oxford Common File Layout (OCFL) specification describes an
> application-independent approach to the storage of digital information in a
> structured, transparent, and predictable manner. It is designed to promote
> long-term object management best practices within digital repositories.
> ([https://ocfl.io/](https://ocfl.io))

## Functionality

Here is a high-level overview of what's working and what's not:

- [x] Both file system and cloud storage backends (via [gocloud](https://gocloud.dev/howto/blob/))
- [x] Storage root creation and validation
- [x] Object creation and validation
- [x] Flexible API for 'staging' object changes between versions.
- [x] Support for OCFL v1.0 and v1.1 
- [x] Reasonable test coverage
- [ ] Ability to purge objects from a storage root
- [ ] Object locking (to prevent simultaneous writes)
- [ ] Consistent, informative error/log messages
- [ ] Well-documented API
- [ ] Stable API

## Command Line Interface

```
A command line tool for working with OCFL Storage Roots and Objects.

Usage:
  gocfl [command]

Available Commands:
  commit      create or update objects in the storage root
  config      print configs
  help        Help about any command
  init-root   initialize an OCFL storage root
  stat        Summary info on storage root or object
  validate    Validates an OCFL Object or Storage Root
```

## Example API Usage

```go
ctx := context.Background()      // functions with i/o take a context
storePath := "test-stage"        // path to storage root
storeFS := ocfl.DirFS(storePath) // in-memory storage root (for testing)
stgFS := stageFS()               // FS abstraction for 'staged' content

// initialize a storage root
if err := ocflv1.InitStore(ctx, storeFS, storePath, nil); err != nil {
    t.Fatal(err)
}

// get a handle for the storage root
store, err := ocflv1.GetStore(ctx, storeFS, storePath, nil)
if err != nil {
    t.Fatal(err)
}

// build an index from `src1` directory for committing
stage, err := ocfl.IndexDir(ctx, stgFS, `src1`, checksum.SHA256)
if err != nil {
    t.Fatal(err)
}

// commit the object with non-standard options
err = store.Commit(ctx, "object-1", stage,
    ocflv1.WithMessage("first commit"),
    ocflv1.WithAlg(digest.SHA256),
    ocflv1.WithContentDir("foo"),
    ocflv1.WithVersionPadding(2),
    ocflv1.WithUser("X", "mailto:nobody@none.com"),
); 
if err != nil {
    t.Fatal("commit failed", err)
}

// fetch the object
obj, err := store.GetObject(ctx, "object-1")
if err != nil {
    t.Fatal("couldn't get the object", err)
}

// validate the object
if err := obj.Validate(ctx); err != nil {
    t.Fatal("object is invalid", err)
}

// fetch the inventory for the object
inv, err := obj.Inventory(ctx)
if err != nil {
    t.Fatal(err)
}
```
