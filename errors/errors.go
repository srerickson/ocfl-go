package errors
// This is generated code. Do not modify. See errors_gen folder.

// OCFLCodeErr represents an OCFL Validation Codes:
// see https://ocfl.io/validation/validation-codes.html
type OCFLCodeErr struct {
	Description string // description from spec
	Code        string // code from spec
	URI         string // reference URI from spec
}


//ErrE001: The OCFL Object Root must not contain files or directories other than those specified in the following sections.
var ErrE001 = OCFLCodeErr{
	Description: "The OCFL Object Root must not contain files or directories other than those specified in the following sections.",
	Code:        "E001",
	URI:         "https://ocfl.io/1.0/spec/#E001",
}

//ErrE002: The version declaration must be formatted according to the NAMASTE specification.
var ErrE002 = OCFLCodeErr{
	Description: "The version declaration must be formatted according to the NAMASTE specification.",
	Code:        "E002",
	URI:         "https://ocfl.io/1.0/spec/#E002",
}

//ErrE003: [The version declaration] must be a file in the base directory of the OCFL Object Root giving the OCFL version in the filename.
var ErrE003 = OCFLCodeErr{
	Description: "[The version declaration] must be a file in the base directory of the OCFL Object Root giving the OCFL version in the filename.",
	Code:        "E003",
	URI:         "https://ocfl.io/1.0/spec/#E003",
}

//ErrE004: The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.
var ErrE004 = OCFLCodeErr{
	Description: "The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.",
	Code:        "E004",
	URI:         "https://ocfl.io/1.0/spec/#E004",
}

//ErrE005: The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.
var ErrE005 = OCFLCodeErr{
	Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.",
	Code:        "E005",
	URI:         "https://ocfl.io/1.0/spec/#E005",
}

//ErrE006: The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification version number.
var ErrE006 = OCFLCodeErr{
	Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification version number.",
	Code:        "E006",
	URI:         "https://ocfl.io/1.0/spec/#E006",
}

//ErrE007: The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).
var ErrE007 = OCFLCodeErr{
	Description: "The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).",
	Code:        "E007",
	URI:         "https://ocfl.io/1.0/spec/#E007",
}

//ErrE008: OCFL Object content must be stored as a sequence of one or more versions.
var ErrE008 = OCFLCodeErr{
	Description: "OCFL Object content must be stored as a sequence of one or more versions.",
	Code:        "E008",
	URI:         "https://ocfl.io/1.0/spec/#E008",
}

//ErrE009: The version number sequence MUST start at 1 and must be continuous without missing integers.
var ErrE009 = OCFLCodeErr{
	Description: "The version number sequence MUST start at 1 and must be continuous without missing integers.",
	Code:        "E009",
	URI:         "https://ocfl.io/1.0/spec/#E009",
}

//ErrE010: The version number sequence must start at 1 and MUST be continuous without missing integers.
var ErrE010 = OCFLCodeErr{
	Description: "The version number sequence must start at 1 and MUST be continuous without missing integers.",
	Code:        "E010",
	URI:         "https://ocfl.io/1.0/spec/#E010",
}

//ErrE011: If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.
var ErrE011 = OCFLCodeErr{
	Description: "If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.",
	Code:        "E011",
	URI:         "https://ocfl.io/1.0/spec/#E011",
}

//ErrE012: All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.
var ErrE012 = OCFLCodeErr{
	Description: "All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.",
	Code:        "E012",
	URI:         "https://ocfl.io/1.0/spec/#E012",
}

//ErrE013: Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.
var ErrE013 = OCFLCodeErr{
	Description: "Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.",
	Code:        "E013",
	URI:         "https://ocfl.io/1.0/spec/#E013",
}

//ErrE014: In all cases, references to files inside version directories from inventory files must use the actual version directory names.
var ErrE014 = OCFLCodeErr{
	Description: "In all cases, references to files inside version directories from inventory files must use the actual version directory names.",
	Code:        "E014",
	URI:         "https://ocfl.io/1.0/spec/#E014",
}

//ErrE015: There must be no other files as children of a version directory, other than an inventory file and a inventory digest.
var ErrE015 = OCFLCodeErr{
	Description: "There must be no other files as children of a version directory, other than an inventory file and a inventory digest.",
	Code:        "E015",
	URI:         "https://ocfl.io/1.0/spec/#E015",
}

//ErrE016: Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.
var ErrE016 = OCFLCodeErr{
	Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.",
	Code:        "E016",
	URI:         "https://ocfl.io/1.0/spec/#E016",
}

//ErrE017: The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).
var ErrE017 = OCFLCodeErr{
	Description: "The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).",
	Code:        "E017",
	URI:         "https://ocfl.io/1.0/spec/#E017",
}

//ErrE018: The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).
var ErrE018 = OCFLCodeErr{
	Description: "The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).",
	Code:        "E018",
	URI:         "https://ocfl.io/1.0/spec/#E018",
}

//ErrE019: If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.
var ErrE019 = OCFLCodeErr{
	Description: "If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.",
	Code:        "E019",
	URI:         "https://ocfl.io/1.0/spec/#E019",
}

//ErrE020: If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.
var ErrE020 = OCFLCodeErr{
	Description: "If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.",
	Code:        "E020",
	URI:         "https://ocfl.io/1.0/spec/#E020",
}

//ErrE021: If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.
var ErrE021 = OCFLCodeErr{
	Description: "If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.",
	Code:        "E021",
	URI:         "https://ocfl.io/1.0/spec/#E021",
}

//ErrE022: OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.
var ErrE022 = OCFLCodeErr{
	Description: "OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.",
	Code:        "E022",
	URI:         "https://ocfl.io/1.0/spec/#E022",
}

//ErrE023: Every file within a version's content directory must be referenced in the manifest section of the inventory.
var ErrE023 = OCFLCodeErr{
	Description: "Every file within a version's content directory must be referenced in the manifest section of the inventory.",
	Code:        "E023",
	URI:         "https://ocfl.io/1.0/spec/#E023",
}

//ErrE024: There must not be empty directories within a version's content directory.
var ErrE024 = OCFLCodeErr{
	Description: "There must not be empty directories within a version's content directory.",
	Code:        "E024",
	URI:         "https://ocfl.io/1.0/spec/#E024",
}

//ErrE025: For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.
var ErrE025 = OCFLCodeErr{
	Description: "For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.",
	Code:        "E025",
	URI:         "https://ocfl.io/1.0/spec/#E025",
}

//ErrE026: For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].
var ErrE026 = OCFLCodeErr{
	Description: "For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].",
	Code:        "E026",
	URI:         "https://ocfl.io/1.0/spec/#E026",
}

//ErrE027: OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.
var ErrE027 = OCFLCodeErr{
	Description: "OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.",
	Code:        "E027",
	URI:         "https://ocfl.io/1.0/spec/#E027",
}

//ErrE028: Optional fixity algorithms that are not supported by a client must be ignored by that client.
var ErrE028 = OCFLCodeErr{
	Description: "Optional fixity algorithms that are not supported by a client must be ignored by that client.",
	Code:        "E028",
	URI:         "https://ocfl.io/1.0/spec/#E028",
}

//ErrE029: SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
var ErrE029 = OCFLCodeErr{
	Description: "SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
	Code:        "E029",
	URI:         "https://ocfl.io/1.0/spec/#E029",
}

//ErrE030: SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
var ErrE030 = OCFLCodeErr{
	Description: "SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
	Code:        "E030",
	URI:         "https://ocfl.io/1.0/spec/#E030",
}

//ErrE031: SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
var ErrE031 = OCFLCodeErr{
	Description: "SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
	Code:        "E031",
	URI:         "https://ocfl.io/1.0/spec/#E031",
}

//ErrE032: [blake2b-512] must be encoded using hex (base16) encoding [RFC4648].
var ErrE032 = OCFLCodeErr{
	Description: "[blake2b-512] must be encoded using hex (base16) encoding [RFC4648].",
	Code:        "E032",
	URI:         "https://ocfl.io/1.0/spec/#E032",
}

//ErrE033: An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.
var ErrE033 = OCFLCodeErr{
	Description: "An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.",
	Code:        "E033",
	URI:         "https://ocfl.io/1.0/spec/#E033",
}

//ErrE034: An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.
var ErrE034 = OCFLCodeErr{
	Description: "An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.",
	Code:        "E034",
	URI:         "https://ocfl.io/1.0/spec/#E034",
}

//ErrE035: The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.
var ErrE035 = OCFLCodeErr{
	Description: "The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.",
	Code:        "E035",
	URI:         "https://ocfl.io/1.0/spec/#E035",
}

//ErrE036: An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]
var ErrE036 = OCFLCodeErr{
	Description: "An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]",
	Code:        "E036",
	URI:         "https://ocfl.io/1.0/spec/#E036",
}

//ErrE037: [id] must be unique in the local context, and should be a URI [RFC3986].
var ErrE037 = OCFLCodeErr{
	Description: "[id] must be unique in the local context, and should be a URI [RFC3986].",
	Code:        "E037",
	URI:         "https://ocfl.io/1.0/spec/#E037",
}

//ErrE038: In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.
var ErrE038 = OCFLCodeErr{
	Description: "In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.",
	Code:        "E038",
	URI:         "https://ocfl.io/1.0/spec/#E038",
}

//ErrE039: [digestAlgorithm] must be the algorithm used in the manifest and state blocks.
var ErrE039 = OCFLCodeErr{
	Description: "[digestAlgorithm] must be the algorithm used in the manifest and state blocks.",
	Code:        "E039",
	URI:         "https://ocfl.io/1.0/spec/#E039",
}

//ErrE040: [head] must be the version directory name with the highest version number.
var ErrE040 = OCFLCodeErr{
	Description: "[head] must be the version directory name with the highest version number.",
	Code:        "E040",
	URI:         "https://ocfl.io/1.0/spec/#E040",
}

//ErrE041: In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.
var ErrE041 = OCFLCodeErr{
	Description: "In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.",
	Code:        "E041",
	URI:         "https://ocfl.io/1.0/spec/#E041",
}

//ErrE042: Content paths within a manifest block must be relative to the OCFL Object Root.
var ErrE042 = OCFLCodeErr{
	Description: "Content paths within a manifest block must be relative to the OCFL Object Root.",
	Code:        "E042",
	URI:         "https://ocfl.io/1.0/spec/#E042",
}

//ErrE043: An OCFL Object Inventory must include a block for storing versions.
var ErrE043 = OCFLCodeErr{
	Description: "An OCFL Object Inventory must include a block for storing versions.",
	Code:        "E043",
	URI:         "https://ocfl.io/1.0/spec/#E043",
}

//ErrE044: This block MUST have the key of versions within the inventory, and it must be a JSON object.
var ErrE044 = OCFLCodeErr{
	Description: "This block MUST have the key of versions within the inventory, and it must be a JSON object.",
	Code:        "E044",
	URI:         "https://ocfl.io/1.0/spec/#E044",
}

//ErrE045: This block must have the key of versions within the inventory, and it MUST be a JSON object.
var ErrE045 = OCFLCodeErr{
	Description: "This block must have the key of versions within the inventory, and it MUST be a JSON object.",
	Code:        "E045",
	URI:         "https://ocfl.io/1.0/spec/#E045",
}

//ErrE046: The keys of [the versions object] must correspond to the names of the version directories used.
var ErrE046 = OCFLCodeErr{
	Description: "The keys of [the versions object] must correspond to the names of the version directories used.",
	Code:        "E046",
	URI:         "https://ocfl.io/1.0/spec/#E046",
}

//ErrE047: Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.
var ErrE047 = OCFLCodeErr{
	Description: "Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.",
	Code:        "E047",
	URI:         "https://ocfl.io/1.0/spec/#E047",
}

//ErrE048: A JSON object to describe one OCFL Version, which must include the following keys: [created, state]
var ErrE048 = OCFLCodeErr{
	Description: "A JSON object to describe one OCFL Version, which must include the following keys: [created, state]",
	Code:        "E048",
	URI:         "https://ocfl.io/1.0/spec/#E048",
}

//ErrE049: [the value of the \"created\" key] must be expressed in the Internet Date/Time Format defined by [RFC3339].
var ErrE049 = OCFLCodeErr{
	Description: "[the value of the \"created\" key] must be expressed in the Internet Date/Time Format defined by [RFC3339].",
	Code:        "E049",
	URI:         "https://ocfl.io/1.0/spec/#E049",
}

//ErrE050: The keys of [the \"state\" JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.
var ErrE050 = OCFLCodeErr{
	Description: "The keys of [the \"state\" JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.",
	Code:        "E050",
	URI:         "https://ocfl.io/1.0/spec/#E050",
}

//ErrE051: The logical path [value of a \"state\" digest key] must be interpreted as a set of one or more path elements joined by a / path separator.
var ErrE051 = OCFLCodeErr{
	Description: "The logical path [value of a \"state\" digest key] must be interpreted as a set of one or more path elements joined by a / path separator.",
	Code:        "E051",
	URI:         "https://ocfl.io/1.0/spec/#E051",
}

//ErrE052: [logical] Path elements must not be ., .., or empty (//).
var ErrE052 = OCFLCodeErr{
	Description: "[logical] Path elements must not be ., .., or empty (//).",
	Code:        "E052",
	URI:         "https://ocfl.io/1.0/spec/#E052",
}

//ErrE053: Additionally, a logical path must not begin or end with a forward slash (/).
var ErrE053 = OCFLCodeErr{
	Description: "Additionally, a logical path must not begin or end with a forward slash (/).",
	Code:        "E053",
	URI:         "https://ocfl.io/1.0/spec/#E053",
}

//ErrE054: The value of the user key must contain a user name key, \"name\" and should contain an address key, \"address\".
var ErrE054 = OCFLCodeErr{
	Description: "The value of the user key must contain a user name key, \"name\" and should contain an address key, \"address\".",
	Code:        "E054",
	URI:         "https://ocfl.io/1.0/spec/#E054",
}

//ErrE055: This block must have the key of fixity within the inventory.
var ErrE055 = OCFLCodeErr{
	Description: "This block must have the key of fixity within the inventory.",
	Code:        "E055",
	URI:         "https://ocfl.io/1.0/spec/#E055",
}

//ErrE056: The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.
var ErrE056 = OCFLCodeErr{
	Description: "The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.",
	Code:        "E056",
	URI:         "https://ocfl.io/1.0/spec/#E056",
}

//ErrE057: The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.
var ErrE057 = OCFLCodeErr{
	Description: "The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.",
	Code:        "E057",
	URI:         "https://ocfl.io/1.0/spec/#E057",
}

//ErrE058: Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.
var ErrE058 = OCFLCodeErr{
	Description: "Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.",
	Code:        "E058",
	URI:         "https://ocfl.io/1.0/spec/#E058",
}

//ErrE059: This value must match the value given for the digestAlgorithm key in the inventory.
var ErrE059 = OCFLCodeErr{
	Description: "This value must match the value given for the digestAlgorithm key in the inventory.",
	Code:        "E059",
	URI:         "https://ocfl.io/1.0/spec/#E059",
}

//ErrE060: The digest sidecar file must contain the digest of the inventory file.
var ErrE060 = OCFLCodeErr{
	Description: "The digest sidecar file must contain the digest of the inventory file.",
	Code:        "E060",
	URI:         "https://ocfl.io/1.0/spec/#E060",
}

//ErrE061: [The digest sidecar file] must follow the format: DIGEST inventory.json
var ErrE061 = OCFLCodeErr{
	Description: "[The digest sidecar file] must follow the format: DIGEST inventory.json",
	Code:        "E061",
	URI:         "https://ocfl.io/1.0/spec/#E061",
}

//ErrE062: The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.
var ErrE062 = OCFLCodeErr{
	Description: "The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.",
	Code:        "E062",
	URI:         "https://ocfl.io/1.0/spec/#E062",
}

//ErrE063: Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.
var ErrE063 = OCFLCodeErr{
	Description: "Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.",
	Code:        "E063",
	URI:         "https://ocfl.io/1.0/spec/#E063",
}

//ErrE064: Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.
var ErrE064 = OCFLCodeErr{
	Description: "Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.",
	Code:        "E064",
	URI:         "https://ocfl.io/1.0/spec/#E064",
}

//ErrE066: Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.
var ErrE066 = OCFLCodeErr{
	Description: "Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.",
	Code:        "E066",
	URI:         "https://ocfl.io/1.0/spec/#E066",
}

//ErrE067: The extensions directory must not contain any files, and no sub-directories other than extension sub-directories.
var ErrE067 = OCFLCodeErr{
	Description: "The extensions directory must not contain any files, and no sub-directories other than extension sub-directories.",
	Code:        "E067",
	URI:         "https://ocfl.io/1.0/spec/#E067",
}

//ErrE068: The specific structure and function of the extension, as well as a declaration of the registered extension name must be defined in one of the following locations: The OCFL Extensions repository OR The Storage Root, as a plain text document directly in the Storage Root.
var ErrE068 = OCFLCodeErr{
	Description: "The specific structure and function of the extension, as well as a declaration of the registered extension name must be defined in one of the following locations: The OCFL Extensions repository OR The Storage Root, as a plain text document directly in the Storage Root.",
	Code:        "E068",
	URI:         "https://ocfl.io/1.0/spec/#E068",
}

//ErrE069: An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.
var ErrE069 = OCFLCodeErr{
	Description: "An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.",
	Code:        "E069",
	URI:         "https://ocfl.io/1.0/spec/#E069",
}

//ErrE070: If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [key, description]
var ErrE070 = OCFLCodeErr{
	Description: "If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [key, description]",
	Code:        "E070",
	URI:         "https://ocfl.io/1.0/spec/#E070",
}

//ErrE071: The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.
var ErrE071 = OCFLCodeErr{
	Description: "The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.",
	Code:        "E071",
	URI:         "https://ocfl.io/1.0/spec/#E071",
}

//ErrE072: The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.
var ErrE072 = OCFLCodeErr{
	Description: "The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.",
	Code:        "E072",
	URI:         "https://ocfl.io/1.0/spec/#E072",
}

//ErrE073: Empty directories MUST NOT appear under a storage root.
var ErrE073 = OCFLCodeErr{
	Description: "Empty directories MUST NOT appear under a storage root.",
	Code:        "E073",
	URI:         "https://ocfl.io/1.0/spec/#E073",
}

//ErrE074: Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple \"buckets\" in an object store - each OCFL Storage Root MUST be independent.
var ErrE074 = OCFLCodeErr{
	Description: "Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple \"buckets\" in an object store - each OCFL Storage Root MUST be independent.",
	Code:        "E074",
	URI:         "https://ocfl.io/1.0/spec/#E074",
}

//ErrE075: The OCFL version declaration MUST be formatted according to the NAMASTE specification.
var ErrE075 = OCFLCodeErr{
	Description: "The OCFL version declaration MUST be formatted according to the NAMASTE specification.",
	Code:        "E075",
	URI:         "https://ocfl.io/1.0/spec/#E075",
}

//ErrE076: [The OCFL version declaration] MUST be a file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.
var ErrE076 = OCFLCodeErr{
	Description: "[The OCFL version declaration] MUST be a file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.",
	Code:        "E076",
	URI:         "https://ocfl.io/1.0/spec/#E076",
}

//ErrE077: [The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.
var ErrE077 = OCFLCodeErr{
	Description: "[The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.",
	Code:        "E077",
	URI:         "https://ocfl.io/1.0/spec/#E077",
}

//ErrE078: [The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.
var ErrE078 = OCFLCodeErr{
	Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.",
	Code:        "E078",
	URI:         "https://ocfl.io/1.0/spec/#E078",
}

//ErrE079: [The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification version number.
var ErrE079 = OCFLCodeErr{
	Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification version number.",
	Code:        "E079",
	URI:         "https://ocfl.io/1.0/spec/#E079",
}

//ErrE080: The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).
var ErrE080 = OCFLCodeErr{
	Description: "The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).",
	Code:        "E080",
	URI:         "https://ocfl.io/1.0/spec/#E080",
}

//ErrE081: OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.
var ErrE081 = OCFLCodeErr{
	Description: "OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.",
	Code:        "E081",
	URI:         "https://ocfl.io/1.0/spec/#E081",
}

//ErrE082: OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.
var ErrE082 = OCFLCodeErr{
	Description: "OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.",
	Code:        "E082",
	URI:         "https://ocfl.io/1.0/spec/#E082",
}

//ErrE083: There MUST be a deterministic mapping from an object identifier to a unique storage path.
var ErrE083 = OCFLCodeErr{
	Description: "There MUST be a deterministic mapping from an object identifier to a unique storage path.",
	Code:        "E083",
	URI:         "https://ocfl.io/1.0/spec/#E083",
}

//ErrE084: Storage hierarchies MUST NOT include files within intermediate directories.
var ErrE084 = OCFLCodeErr{
	Description: "Storage hierarchies MUST NOT include files within intermediate directories.",
	Code:        "E084",
	URI:         "https://ocfl.io/1.0/spec/#E084",
}

//ErrE085: Storage hierarchies MUST be terminated by OCFL Object Roots.
var ErrE085 = OCFLCodeErr{
	Description: "Storage hierarchies MUST be terminated by OCFL Object Roots.",
	Code:        "E085",
	URI:         "https://ocfl.io/1.0/spec/#E085",
}

//ErrE086: The storage root extensions directory MUST conform to the same guidelines and limitations as those defined for object extensions.
var ErrE086 = OCFLCodeErr{
	Description: "The storage root extensions directory MUST conform to the same guidelines and limitations as those defined for object extensions.",
	Code:        "E086",
	URI:         "https://ocfl.io/1.0/spec/#E086",
}

//ErrE087: An OCFL validator MUST ignore any files in the storage root it does not understand.
var ErrE087 = OCFLCodeErr{
	Description: "An OCFL validator MUST ignore any files in the storage root it does not understand.",
	Code:        "E087",
	URI:         "https://ocfl.io/1.0/spec/#E087",
}

//ErrE088: An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.
var ErrE088 = OCFLCodeErr{
	Description: "An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.",
	Code:        "E088",
	URI:         "https://ocfl.io/1.0/spec/#E088",
}

//ErrE089: If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.
var ErrE089 = OCFLCodeErr{
	Description: "If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.",
	Code:        "E089",
	URI:         "https://ocfl.io/1.0/spec/#E089",
}

//ErrE090: Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierachies.
var ErrE090 = OCFLCodeErr{
	Description: "Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierachies.",
	Code:        "E090",
	URI:         "https://ocfl.io/1.0/spec/#E090",
}

//ErrE091: Filesystems MUST preserve the case of OCFL filepaths and filenames.
var ErrE091 = OCFLCodeErr{
	Description: "Filesystems MUST preserve the case of OCFL filepaths and filenames.",
	Code:        "E091",
	URI:         "https://ocfl.io/1.0/spec/#E091",
}

//ErrE092: The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.
var ErrE092 = OCFLCodeErr{
	Description: "The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.",
	Code:        "E092",
	URI:         "https://ocfl.io/1.0/spec/#E092",
}

//ErrE093: Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.
var ErrE093 = OCFLCodeErr{
	Description: "Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.",
	Code:        "E093",
	URI:         "https://ocfl.io/1.0/spec/#E093",
}

//ErrE094: The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.
var ErrE094 = OCFLCodeErr{
	Description: "The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.",
	Code:        "E094",
	URI:         "https://ocfl.io/1.0/spec/#E094",
}

//ErrE095: Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.
var ErrE095 = OCFLCodeErr{
	Description: "Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.",
	Code:        "E095",
	URI:         "https://ocfl.io/1.0/spec/#E095",
}

//ErrE096: As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.
var ErrE096 = OCFLCodeErr{
	Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.",
	Code:        "E096",
	URI:         "https://ocfl.io/1.0/spec/#E096",
}

//ErrE097: As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.
var ErrE097 = OCFLCodeErr{
	Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.",
	Code:        "E097",
	URI:         "https://ocfl.io/1.0/spec/#E097",
}

//ErrE098: The content path must be interpreted as a set of one or more path elements joined by a / path separator.
var ErrE098 = OCFLCodeErr{
	Description: "The content path must be interpreted as a set of one or more path elements joined by a / path separator.",
	Code:        "E098",
	URI:         "https://ocfl.io/1.0/spec/#E098",
}

//ErrE099: [content] path elements must not be ., .., or empty (//).
var ErrE099 = OCFLCodeErr{
	Description: "[content] path elements must not be ., .., or empty (//).",
	Code:        "E099",
	URI:         "https://ocfl.io/1.0/spec/#E099",
}

//ErrE100: A content path must not begin or end with a forward slash (/).
var ErrE100 = OCFLCodeErr{
	Description: "A content path must not begin or end with a forward slash (/).",
	Code:        "E100",
	URI:         "https://ocfl.io/1.0/spec/#E100",
}

//ErrE101: Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.
var ErrE101 = OCFLCodeErr{
	Description: "Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.",
	Code:        "E101",
	URI:         "https://ocfl.io/1.0/spec/#E101",
}

//ErrE102: An inventory file must not contain keys that are not specified.
var ErrE102 = OCFLCodeErr{
	Description: "An inventory file must not contain keys that are not specified.",
	Code:        "E102",
	URI:         "https://ocfl.io/1.0/spec/#E102",
}

//ErrW001: Implementations SHOULD use version directory names constructed without zero-padding the version number, ie. v1, v2, v3, etc.
var ErrW001 = OCFLCodeErr{
	Description: "Implementations SHOULD use version directory names constructed without zero-padding the version number, ie. v1, v2, v3, etc.",
	Code:        "W001",
	URI:         "https://ocfl.io/1.0/spec/#W001",
}

//ErrW002: The version directory SHOULD NOT contain any directories other than the designated content sub-directory. Once created, the contents of a version directory are expected to be immutable.
var ErrW002 = OCFLCodeErr{
	Description: "The version directory SHOULD NOT contain any directories other than the designated content sub-directory. Once created, the contents of a version directory are expected to be immutable.",
	Code:        "W002",
	URI:         "https://ocfl.io/1.0/spec/#W002",
}

//ErrW003: Version directories must contain a designated content sub-directory if the version contains files to be preserved, and SHOULD NOT contain this sub-directory otherwise.
var ErrW003 = OCFLCodeErr{
	Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and SHOULD NOT contain this sub-directory otherwise.",
	Code:        "W003",
	URI:         "https://ocfl.io/1.0/spec/#W003",
}

//ErrW004: For content-addressing, OCFL Objects SHOULD use sha512.
var ErrW004 = OCFLCodeErr{
	Description: "For content-addressing, OCFL Objects SHOULD use sha512.",
	Code:        "W004",
	URI:         "https://ocfl.io/1.0/spec/#W004",
}

//ErrW005: The OCFL Object Inventory id SHOULD be a URI.
var ErrW005 = OCFLCodeErr{
	Description: "The OCFL Object Inventory id SHOULD be a URI.",
	Code:        "W005",
	URI:         "https://ocfl.io/1.0/spec/#W005",
}

//ErrW007: In the OCFL Object Inventory, the JSON object describing an OCFL Version, SHOULD include the message and user keys.
var ErrW007 = OCFLCodeErr{
	Description: "In the OCFL Object Inventory, the JSON object describing an OCFL Version, SHOULD include the message and user keys.",
	Code:        "W007",
	URI:         "https://ocfl.io/1.0/spec/#W007",
}

//ErrW008: In the OCFL Object Inventory, in the version block, the value of the user key SHOULD contain an address key, address.
var ErrW008 = OCFLCodeErr{
	Description: "In the OCFL Object Inventory, in the version block, the value of the user key SHOULD contain an address key, address.",
	Code:        "W008",
	URI:         "https://ocfl.io/1.0/spec/#W008",
}

//ErrW009: In the OCFL Object Inventory, in the version block, the address value SHOULD be a URI: either a mailto URI [RFC6068] with the e-mail address of the user or a URL to a personal identifier, e.g., an ORCID iD.
var ErrW009 = OCFLCodeErr{
	Description: "In the OCFL Object Inventory, in the version block, the address value SHOULD be a URI: either a mailto URI [RFC6068] with the e-mail address of the user or a URL to a personal identifier, e.g., an ORCID iD.",
	Code:        "W009",
	URI:         "https://ocfl.io/1.0/spec/#W009",
}

//ErrW010: In addition to the inventory in the OCFL Object Root, every version directory SHOULD include an inventory file that is an Inventory of all content for versions up to and including that particular version.
var ErrW010 = OCFLCodeErr{
	Description: "In addition to the inventory in the OCFL Object Root, every version directory SHOULD include an inventory file that is an Inventory of all content for versions up to and including that particular version.",
	Code:        "W010",
	URI:         "https://ocfl.io/1.0/spec/#W010",
}

//ErrW011: In the case that prior version directories include an inventory file, the values of the created, message and user keys in each version block in each prior inventory file SHOULD have the same values as the corresponding keys in the corresponding version block in the current inventory file.
var ErrW011 = OCFLCodeErr{
	Description: "In the case that prior version directories include an inventory file, the values of the created, message and user keys in each version block in each prior inventory file SHOULD have the same values as the corresponding keys in the corresponding version block in the current inventory file.",
	Code:        "W011",
	URI:         "https://ocfl.io/1.0/spec/#W011",
}

//ErrW012: Implementers SHOULD use the logs directory, if present, for storing files that contain a record of actions taken on the object.
var ErrW012 = OCFLCodeErr{
	Description: "Implementers SHOULD use the logs directory, if present, for storing files that contain a record of actions taken on the object.",
	Code:        "W012",
	URI:         "https://ocfl.io/1.0/spec/#W012",
}

//ErrW013: In an OCFL Object, extension sub-directories SHOULD be named according to a registered extension name.
var ErrW013 = OCFLCodeErr{
	Description: "In an OCFL Object, extension sub-directories SHOULD be named according to a registered extension name.",
	Code:        "W013",
	URI:         "https://ocfl.io/1.0/spec/#W013",
}

//ErrW014: Storage hierarchies within the same OCFL Storage Root SHOULD use just one layout pattern.
var ErrW014 = OCFLCodeErr{
	Description: "Storage hierarchies within the same OCFL Storage Root SHOULD use just one layout pattern.",
	Code:        "W014",
	URI:         "https://ocfl.io/1.0/spec/#W014",
}

//ErrW015: Storage hierarchies within the same OCFL Storage Root SHOULD consistently use either a directory hierarchy of OCFL Objects or top-level OCFL Objects.
var ErrW015 = OCFLCodeErr{
	Description: "Storage hierarchies within the same OCFL Storage Root SHOULD consistently use either a directory hierarchy of OCFL Objects or top-level OCFL Objects.",
	Code:        "W015",
	URI:         "https://ocfl.io/1.0/spec/#W015",
}
