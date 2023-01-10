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