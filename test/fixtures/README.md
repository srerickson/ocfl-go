# OCFL Test Fixtures

All fixtures are drafts for a proposed 1.0 release of OCFL. Everything in this repository is organized by OCFL version number as the top-level directory.

## OCFL v1.0

See error and warning codes in https://github.com/OCFL/spec/blob/master/validation/validation-codes.md

Within the `1.0` directory there are three directories:

### `good-objects`

This directory contains valid OCFL objects. Each directory is the OCFL object root of the valid object.

### `content`

This directory contains content that can be used to test the construction of OCFL objects, as well as the subsequent extraction of data from OCFL objects. Each fixture in this directory contains a short README file describing the features of the fixture.

Some of these fixtures correspond to a valid OCFL object provided in the `objects` directory. These correspond by name. For example, `content/spec-ex-full` maps to `objects/spec-ex-full`, and `content/cf1` maps to `objects/of1`.

### `warn-objects`

This directory contains OCFL objects that should trigger warnings. The [warning codes](https://github.com/OCFL/spec/blob/master/validation/validation-codes.md#warnings--corresponding-with-should-in-specification) are used a prefixes to the object directory name, e.g. `W001_W004_W005_zero_padded_versions` is expected to generate warnings `W001`, `W004` and `W005`.

### `bad-objects`

This directory contains invalid OCFL objects.

## Update policy

  * All proposed changes must be made as [pull-requests](https://github.com/OCFL/fixtures/pulls)
  * Two [OCFL editors](https://github.com/orgs/OCFL/teams/editors) must approve the pull-request (one may be the pull-request creator who's approval is implicit)
  * After 24 hours, if there are no editors objecting, any editor except the pull-request author can merge
