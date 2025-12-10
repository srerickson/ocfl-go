# OCFL for Go

<a href="https://godoc.org/github.com/srerickson/ocfl-go"><img src="https://godoc.org/github.com/srerickson/ocfl-go?status.svg" alt="godocs"/></a>
<a href="https://doi.org/10.5281/zenodo.15212966"><img src="https://zenodo.org/badge/DOI/10.5281/zenodo.15212966.svg" alt="doi"/></a>

This is an implementation of the [Oxford Common File Layout](https://ocfl.io/)
for [Go](https://go.dev). The module can be used in Go programs to support
operations on OCFL storage roots and objects. It supports the local file system
or s3 storage backends. Several complete [example programs](examples) are
included.

> [!WARNING]  
> The API is under heavy development and will have constant breaking changes.

This module is used in the following projects:

- [ocfl-tools](https://github.com/srerickson/ocfl-tools) (command line tools)
- [ocfl-services](https://github.com/srerickson/ocfl-services) (web services)

## Development

Requires Go >= v1.24.
