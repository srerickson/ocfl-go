# OCFL Community Extension 0008: Schema Registry

  * **Extension Name:** `0008-schema-registry`
  * **Authors:** P. Cornwell, D. Granville
  * **Minimum OCFL Version:** 1.0
  * **OCFL Community Extensions Version:** 1.0
  * **Related to:** n/a

## Overview

An OCFL object will typically contain metadata serialised in JSON or XML files. These nominally conform to one or more schemata referenced as external JSONSchema/XSD/DTD files.

In order for an OCFL root to represent a self-contained repository, the specific versions of each schema referenced must be available in order to validate the content of OCFL objects. It is currently assumed that such schemata are maintained independently in external files and remain accessible via URLs. However, maintaining a local copy of schemata within the OCFL root is essential where long-term preservation is a goal. External online references must not be relied upon in the long-term, due to the possibility of evolution of technical ecosystems and of organizational change.

This extension stores a single copy of each schema referenced by the content of OCFL objects within the root: it avoids the need to store a copy of schemata redundantly in each OCFL object. It also provides a convenient reference point for archive management software and future users inspecting the contents of the OCFL object.

We describe a standardised layout for a schema directory (hereinafter schema registry), where this extension stores all schemata referenced throughout the OCFL root. This schema registry must be implemented by creating and maintaining the following items:

* A `config.json` file containing configuration parameters for the registry.
* The `schemata` directory containing a reference copy of each schema in use.
* The `schema_inventory.json` file providing an index of and checksums for the stored schemata.
* A sidecar file `schema_inventory.json.sha512` (or other configured digest) containing the checksum of `schema_inventory.json`, in the same manner as OCFL inventory files.

Example file content and structures are shown in the Examples section below.

## Parameters

Configuration is done by setting values in `config.json` at the top level of the extension's directory. The keys expected are:

* Name: `extensionName`
  * Description: String identifying the extension.
  * Type: String
  * Constraints: Must be `0008-schema-registry`.
  * Default: `0008-schema-registry`

* Name: `identifierDigestAlgorithm`
  * Description: Algorithm used for calculating safe filenames from schema identifiers.
  * Type: String
  * Constraints: Must be a valid digest algorithm returning strings which are safe file names on the target file system.
  * Default: `md5`

* Name: `digestAlgorithm`
  * Description: Digest algorithm used for calculating fixity of the schema inventory and stored schema files.
  * Type: String
  * Constraints: Must be a valid digest algorithm.
  * Default: The same value use elsewhere in the OCFL for integrity checking.

An example `config.json` in included in the Examples section below.

## schema_inventory.json

A manifest of the registered schemata must be maintained in `schema_inventory.json`. Implementations of this extension must treat integrity verification of the stored schemata with the same priority as all primary OCFL objects.

* Name: `manifest`
  * Description: Object containing manifest entries. Indexed by the stored filename - ie the digest of the schema's identifier.
  * Type: Object
  * Constraints: Must contain one entry per registered schema; must contain 2 sub-keys documented below.
  * Default: Not applicable

### manifest entry properties for `schema_inventory.json`

* Name: `digest`
  * Description: The digest of the stored schema file, used for integrity verification.
  * Type: String
  * Constraints: Must be the digest of the stored file, as calculated using the configured `digestAlgorithm`.
  * Default: Not applicable

* Name: `identifier`
  * Description: Original identifier string for the schema.
  * Type: String
  * Constraints: Must be the string that was hashed with `identifierDigestAlgorithm` to produce the stored file name.
  * Default: Not applicable

## Implementation

In an OCFL root where this extension is configured, tools reading the OCFL objects may be aware of local copies of schemata. Accessing local schemata may reduce latency and provide a performance benefit. Thus, OCFL reading applications relying on schemata (eg for validation) may wish to read these from the schema registry.

A process must be implemented which ensures all OCFL objects/versions written are checked for schema references. Where not already registered, the schema must be retrieved and registered as described below. Since schemata are typically identified by the URL of the specific version referenced, the digest of the schema's identifier (as configured by `identifierDigestAlgorithm`) must be used as a filename. MD5 may be sufficient, other algorithms may be configured.

The values of `digestAlgorithm` and `identifierDigestAlgorithm` should not be changed once the registry is initialised. If changing the digest is unavoidable, all existing entries in the registry must be updated to the new algorithm(s).

Two possible implementation cases are considered:

### Where an OCFL root is being generated as a snapshot for export or archival purposes.

In this case, it is reasonable to build or check the completeness of the schema registry as each OCFL object is processed for backup. Then the exported root will contain all necessary schemata.

### Where an OCFL root provides the persistence layer for a live repository

In this case, a background process may discover or be informed of any new OCFL objects/versions. This process can asynchronously process the object contents, retrieve the required schemata and perform necessary registrations with the schema registry.

The values of `digestAlgorithm` and `identifierDigestAlgorithm` should not be changed once the registry is initialised. If changing the digest is unavoidable, all existing entries in the registry must be updated to the new algorithm(s).

### Registering a schema with the extension

* New OCFL objects/versions must be inspected for JSON, XML files and references to external schema ($schema / XML-DTD etc) are extracted. This may occur periodically or be triggered by creation of a new or updated OCFL object.
* For each referenced schema:
  * The local filename is derived by hashing the schema's identifier using the configured `identifierDigestAlgorithm`.
  * The manifest object in `schema_inventory.json` must be checked for this key.
    * If this identifier is not already present in the schema repository:
      * A copy of the remote schema must be retrieved, and stored in the `schemata` directory with the derived local filename.
      * The local copy's digest must be calculated with the configured digestAlgorithm.
      * The `schema_inventory.json` must updated:
        * a new key must be created in the manifest object, with the properties `digest` and `identifier`.
      * The `schema_inventory.json`’s sidecar file must also be updated with the appropriate checksum.
    * If the schema's checksum is already registered, the identifiers must be compared to exclude the possibility of hash-collision (malicious or otherwise).
      * If the identifiers match, the schema is already registered and no further action needs to be taken.
      * If the identifiers differ, an exception must be raised.


## Example

Given OCFL objects:

item1
: with descriptive metadata in `content/item1.xml`

```xml
<!DOCTYPE rdf:RDF SYSTEM "http://dublincore.org/specifications/dublin-core/dcmes-xml/2001-04-11/dcmes-xml-dtd.dtd">
...
```
item2
: with descriptive metadata in `content/item2.json`

```json
{
  "$schema" : "http://schemata.hasdai.org/historic-persons/historic-person-entry-v1.0.0.json"
...
```

An example state of the registry is shown below, with local copies of the named schemata stored with their derived filenames

### `config.json`

```json
{
  "extensionName" : "0008-schema-registry",
  "identifierDigestAlgorithm": "md5",
  "digestAlgorithm" : "sha512"
}
```

### `schema_inventory.json`

```json
{
  "manifest" : {
    "40cdd53d9a263e5466b8954d82d23daa" : {
     "digest" : "91da2b...9c8",
     "identifier" : "http://dublincore.org/specifications/dublin-core/dcmes-xml/2001-04-11/dcmes-xml-dtd.dtd"
    },
    "95d751340dcdc784fd759dbc7ddb9633" : {
     "digest" : "31f53b...7ff",
     "identifier" : "http://schemata.hasdai.org/historic-persons/historic-person-entry-v1.0.0.json"
    }
  }
}
```

### OCFL root tree

```
[storage_root]
  ├── 0=ocfl_1.0
  ├── ocfl_1.0.txt
  ├── ocfl_layout.json
  ├── extensions
  │   └── 0008-schema-registry
  │       └── config.json
  |       └── schema_inventory.json
  |       └── schema_inventory.json.sha512
  │       └── schemata
  │           └── 40cdd53d9a263e5466b8954d82d23daa
  │           └── 95d751340dcdc784fd759dbc7ddb9633
  ├── 0de
  |   └── 45c
  |       └── f24
  |           └── item1
  │               └── 0=ocfl_object_1.0
  │                   └── inventory.json
  │                   └── inventory.json.sha512
  │                   └── v1
  │                       └── inventory.json
  │                       └── inventory.json.sha512
  │                       └── content
  │                           └── item1.xml
  ...
```
