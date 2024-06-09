# An OCFL implementation for Go

<a href="https://godoc.org/github.com/srerickson/ocfl-go">
    <img src="https://godoc.org/github.com/srerickson/ocfl-go?status.svg" alt="godocs"/>
</a>
<a href="https://goreportcard.com/report/github.com/srerickson/ocfl-go">
    <img src="https://goreportcard.com/badge/github.com/srerickson/ocfl-go">
</a>

This is an implementation of the Oxford Common File Layout
([OCFL](https://ocfl.io/)) for Go. The API is under heavy
development and will have constant breaking changes.

## What is OCFL?

> This Oxford Common File Layout (OCFL) specification describes an
> application-independent approach to the storage of digital information in a
> structured, transparent, and predictable manner. It is designed to promote
> long-term object management best practices within digital repositories.
> ([https://ocfl.io/](https://ocfl.io))

## Functionality

Here is a high-level overview of what's working and what's not:

- [x] Filesystme and S3 backends
  - [x] S3: support writing/copying large files (>5GiB).
- [x] Storage root creation and validation
- [x] Object creation and validation
- [x] Flexible API for 'staging' object changes between versions.
- [x] Support for OCFL v1.0 and v1.1
- [x] Reasonable test coverage
- [x] Ability to purge objects from a storage root
- [ ] Consistent, informative error/log messages
- [ ] Well-documented API
- [ ] Stable API



## API overview


### ocfl.StorageRoot

```go
// create a new store
store, err := ocfl.InitStore(ctx, fsys, path, spec, description, extensions ... extension.Extension)

// get an existing store
store, err := ocfl.GetStore(ctx, fsys, path)

// set storage root layout if necessary (needed to access objects)
store.Layout = customLayout


```

### ocfl.Object

```go
// object reference from store with storage layout
obj, err := store.NewObject(ctx, id)

// require object doesn't exist (i.e., before creating)
obj, err := store.NewObject(ctx, id,
  ocfl.MustNotExist())

// options for a new object
obj.CreateWith() 

// remove an object
err := store.RemoveObject(ctx, id)

// commit a new version
stage, err := ocfl.StageDir(ctx, fsys, path, `sha512`)
result, err := obj.Commit(ctx, stage, user, message)




// create new object reference, and require that it exists
obj, err := store.NewObject(ctx, id, 
  ocfl.MustExist(),
  ocfl.SkipInventoryValidation(),
  ocfl.SkipExtensions())


// access contents of an object
ver := obj.Version(0)
for name := range ver.Files {
  ...
}
```



## Development

Requires go >= 1.21.
