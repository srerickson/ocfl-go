package codes

// This is generated code. Do not modify. See gen folder.

import (
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/validation"
)

// E001: The OCFL Object Root must not contain files or directories other than those specified in the following sections.
var E001 = validation.NewCode("E001",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The OCFL Object Root must not contain files or directories other than those specified in the following sections.",
			URL:         "https://ocfl.io/1.0/spec/#E001",
		},
		ocfl.Spec1_1: {
			Description: "The OCFL Object Root must not contain files or directories other than those specified in the following sections.",
			URL:         "https://ocfl.io/1.1/spec/#E001",
		},
	})

// E002: The version declaration must be formatted according to the NAMASTE specification.
var E002 = validation.NewCode("E002",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The version declaration must be formatted according to the NAMASTE specification.",
			URL:         "https://ocfl.io/1.0/spec/#E002",
		},
		ocfl.Spec1_1: {
			Description: "The version declaration must be formatted according to the NAMASTE specification.",
			URL:         "https://ocfl.io/1.1/spec/#E002",
		},
	})

// E003: [The version declaration] must be a file in the base directory of the OCFL Object Root giving the OCFL version in the filename.
var E003 = validation.NewCode("E003",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[The version declaration] must be a file in the base directory of the OCFL Object Root giving the OCFL version in the filename.",
			URL:         "https://ocfl.io/1.0/spec/#E003",
		},
		ocfl.Spec1_1: {
			Description: "There must be exactly one version declaration file in the base directory of the OCFL Object Root giving the OCFL version in the filename.",
			URL:         "https://ocfl.io/1.1/spec/#E003",
		},
	})

// E004: The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification ocfl.Number.
var E004 = validation.NewCode("E004",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E004",
		},
		ocfl.Spec1_1: {
			Description: "The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E004",
		},
	})

// E005: The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification ocfl.Number.
var E005 = validation.NewCode("E005",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E005",
		},
		ocfl.Spec1_1: {
			Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E005",
		},
	})

// E006: The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification ocfl.Number.
var E006 = validation.NewCode("E006",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E006",
		},
		ocfl.Spec1_1: {
			Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E006",
		},
	})

// E007: The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).
var E007 = validation.NewCode("E007",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).",
			URL:         "https://ocfl.io/1.0/spec/#E007",
		},
		ocfl.Spec1_1: {
			Description: "The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).",
			URL:         "https://ocfl.io/1.1/spec/#E007",
		},
	})

// E008: OCFL Object content must be stored as a sequence of one or more versions.
var E008 = validation.NewCode("E008",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "OCFL Object content must be stored as a sequence of one or more versions.",
			URL:         "https://ocfl.io/1.0/spec/#E008",
		},
		ocfl.Spec1_1: {
			Description: "OCFL Object content must be stored as a sequence of one or more versions.",
			URL:         "https://ocfl.io/1.1/spec/#E008",
		},
	})

// E009: The ocfl.Number sequence MUST start at 1 and must be continuous without missing integers.
var E009 = validation.NewCode("E009",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The ocfl.Number sequence MUST start at 1 and must be continuous without missing integers.",
			URL:         "https://ocfl.io/1.0/spec/#E009",
		},
		ocfl.Spec1_1: {
			Description: "The version number sequence MUST start at 1 and must be continuous without missing integers.",
			URL:         "https://ocfl.io/1.1/spec/#E009",
		},
	})

// E010: The ocfl.Number sequence must start at 1 and MUST be continuous without missing integers.
var E010 = validation.NewCode("E010",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The ocfl.Number sequence must start at 1 and MUST be continuous without missing integers.",
			URL:         "https://ocfl.io/1.0/spec/#E010",
		},
		ocfl.Spec1_1: {
			Description: "The version number sequence must start at 1 and MUST be continuous without missing integers.",
			URL:         "https://ocfl.io/1.1/spec/#E010",
		},
	})

// E011: If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.
var E011 = validation.NewCode("E011",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.",
			URL:         "https://ocfl.io/1.0/spec/#E011",
		},
		ocfl.Spec1_1: {
			Description: "If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.",
			URL:         "https://ocfl.io/1.1/spec/#E011",
		},
	})

// E012: All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.
var E012 = validation.NewCode("E012",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.",
			URL:         "https://ocfl.io/1.0/spec/#E012",
		},
		ocfl.Spec1_1: {
			Description: "All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.",
			URL:         "https://ocfl.io/1.1/spec/#E012",
		},
	})

// E013: Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.
var E013 = validation.NewCode("E013",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.",
			URL:         "https://ocfl.io/1.0/spec/#E013",
		},
		ocfl.Spec1_1: {
			Description: "Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.",
			URL:         "https://ocfl.io/1.1/spec/#E013",
		},
	})

// E014: In all cases, references to files inside version directories from inventory files must use the actual version directory names.
var E014 = validation.NewCode("E014",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In all cases, references to files inside version directories from inventory files must use the actual version directory names.",
			URL:         "https://ocfl.io/1.0/spec/#E014",
		},
		ocfl.Spec1_1: {
			Description: "In all cases, references to files inside version directories from inventory files must use the actual version directory names.",
			URL:         "https://ocfl.io/1.1/spec/#E014",
		},
	})

// E015: There must be no other files as children of a version directory, other than an inventory file and a inventory digest.
var E015 = validation.NewCode("E015",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "There must be no other files as children of a version directory, other than an inventory file and a inventory digest.",
			URL:         "https://ocfl.io/1.0/spec/#E015",
		},
		ocfl.Spec1_1: {
			Description: "There must be no other files as children of a version directory, other than an inventory file and a inventory digest.",
			URL:         "https://ocfl.io/1.1/spec/#E015",
		},
	})

// E016: Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.
var E016 = validation.NewCode("E016",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.",
			URL:         "https://ocfl.io/1.0/spec/#E016",
		},
		ocfl.Spec1_1: {
			Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.",
			URL:         "https://ocfl.io/1.1/spec/#E016",
		},
	})

// E017: The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).
var E017 = validation.NewCode("E017",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).",
			URL:         "https://ocfl.io/1.0/spec/#E017",
		},
		ocfl.Spec1_1: {
			Description: "The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).",
			URL:         "https://ocfl.io/1.1/spec/#E017",
		},
	})

// E018: The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).
var E018 = validation.NewCode("E018",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).",
			URL:         "https://ocfl.io/1.0/spec/#E018",
		},
		ocfl.Spec1_1: {
			Description: "The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).",
			URL:         "https://ocfl.io/1.1/spec/#E018",
		},
	})

// E019: If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.
var E019 = validation.NewCode("E019",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.",
			URL:         "https://ocfl.io/1.0/spec/#E019",
		},
		ocfl.Spec1_1: {
			Description: "If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.",
			URL:         "https://ocfl.io/1.1/spec/#E019",
		},
	})

// E020: If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.
var E020 = validation.NewCode("E020",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.",
			URL:         "https://ocfl.io/1.0/spec/#E020",
		},
		ocfl.Spec1_1: {
			Description: "If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.",
			URL:         "https://ocfl.io/1.1/spec/#E020",
		},
	})

// E021: If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.
var E021 = validation.NewCode("E021",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.",
			URL:         "https://ocfl.io/1.0/spec/#E021",
		},
		ocfl.Spec1_1: {
			Description: "If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.",
			URL:         "https://ocfl.io/1.1/spec/#E021",
		},
	})

// E022: OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.
var E022 = validation.NewCode("E022",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.",
			URL:         "https://ocfl.io/1.0/spec/#E022",
		},
		ocfl.Spec1_1: {
			Description: "OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.",
			URL:         "https://ocfl.io/1.1/spec/#E022",
		},
	})

// E023: Every file within a version's content directory must be referenced in the manifest section of the inventory.
var E023 = validation.NewCode("E023",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Every file within a version's content directory must be referenced in the manifest section of the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E023",
		},
		ocfl.Spec1_1: {
			Description: "Every file within a version's content directory must be referenced in the manifest section of the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E023",
		},
	})

// E024: There must not be empty directories within a version's content directory.
var E024 = validation.NewCode("E024",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "There must not be empty directories within a version's content directory.",
			URL:         "https://ocfl.io/1.0/spec/#E024",
		},
		ocfl.Spec1_1: {
			Description: "There must not be empty directories within a version's content directory.",
			URL:         "https://ocfl.io/1.1/spec/#E024",
		},
	})

// E025: For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.
var E025 = validation.NewCode("E025",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.",
			URL:         "https://ocfl.io/1.0/spec/#E025",
		},
		ocfl.Spec1_1: {
			Description: "For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.",
			URL:         "https://ocfl.io/1.1/spec/#E025",
		},
	})

// E026: For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].
var E026 = validation.NewCode("E026",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].",
			URL:         "https://ocfl.io/1.0/spec/#E026",
		},
		ocfl.Spec1_1: {
			Description: "For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].",
			URL:         "https://ocfl.io/1.1/spec/#E026",
		},
	})

// E027: OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.
var E027 = validation.NewCode("E027",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.",
			URL:         "https://ocfl.io/1.0/spec/#E027",
		},
		ocfl.Spec1_1: {
			Description: "OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.",
			URL:         "https://ocfl.io/1.1/spec/#E027",
		},
	})

// E028: Optional fixity algorithms that are not supported by a client must be ignored by that client.
var E028 = validation.NewCode("E028",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Optional fixity algorithms that are not supported by a client must be ignored by that client.",
			URL:         "https://ocfl.io/1.0/spec/#E028",
		},
		ocfl.Spec1_1: {
			Description: "Optional fixity algorithms that are not supported by a client must be ignored by that client.",
			URL:         "https://ocfl.io/1.1/spec/#E028",
		},
	})

// E029: SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
var E029 = validation.NewCode("E029",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.0/spec/#E029",
		},
		ocfl.Spec1_1: {
			Description: "SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.1/spec/#E029",
		},
	})

// E030: SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
var E030 = validation.NewCode("E030",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.0/spec/#E030",
		},
		ocfl.Spec1_1: {
			Description: "SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.1/spec/#E030",
		},
	})

// E031: SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
var E031 = validation.NewCode("E031",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.0/spec/#E031",
		},
		ocfl.Spec1_1: {
			Description: "SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.1/spec/#E031",
		},
	})

// E032: [blake2b-512] must be encoded using hex (base16) encoding [RFC4648].
var E032 = validation.NewCode("E032",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[blake2b-512] must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.0/spec/#E032",
		},
		ocfl.Spec1_1: {
			Description: "[blake2b-512] must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.1/spec/#E032",
		},
	})

// E033: An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.
var E033 = validation.NewCode("E033",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.",
			URL:         "https://ocfl.io/1.0/spec/#E033",
		},
		ocfl.Spec1_1: {
			Description: "An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.",
			URL:         "https://ocfl.io/1.1/spec/#E033",
		},
	})

// E034: An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.
var E034 = validation.NewCode("E034",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.",
			URL:         "https://ocfl.io/1.0/spec/#E034",
		},
		ocfl.Spec1_1: {
			Description: "An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.",
			URL:         "https://ocfl.io/1.1/spec/#E034",
		},
	})

// E035: The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.
var E035 = validation.NewCode("E035",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E035",
		},
		ocfl.Spec1_1: {
			Description: "The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E035",
		},
	})

// E036: An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]
var E036 = validation.NewCode("E036",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]",
			URL:         "https://ocfl.io/1.0/spec/#E036",
		},
		ocfl.Spec1_1: {
			Description: "An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]",
			URL:         "https://ocfl.io/1.1/spec/#E036",
		},
	})

// E037: [id] must be unique in the local context, and should be a URI [RFC3986].
var E037 = validation.NewCode("E037",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[id] must be unique in the local context, and should be a URI [RFC3986].",
			URL:         "https://ocfl.io/1.0/spec/#E037",
		},
		ocfl.Spec1_1: {
			Description: "[id] must be unique in the local context, and should be a URI [RFC3986].",
			URL:         "https://ocfl.io/1.1/spec/#E037",
		},
	})

// E038: In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.
var E038 = validation.NewCode("E038",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.",
			URL:         "https://ocfl.io/1.0/spec/#E038",
		},
		ocfl.Spec1_1: {
			Description: "In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.",
			URL:         "https://ocfl.io/1.1/spec/#E038",
		},
	})

// E039: [digestAlgorithm] must be the algorithm used in the manifest and state blocks.
var E039 = validation.NewCode("E039",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[digestAlgorithm] must be the algorithm used in the manifest and state blocks.",
			URL:         "https://ocfl.io/1.0/spec/#E039",
		},
		ocfl.Spec1_1: {
			Description: "[digestAlgorithm] must be the algorithm used in the manifest and state blocks.",
			URL:         "https://ocfl.io/1.1/spec/#E039",
		},
	})

// E040: [head] must be the version directory name with the highest ocfl.Number.
var E040 = validation.NewCode("E040",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[head] must be the version directory name with the highest ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E040",
		},
		ocfl.Spec1_1: {
			Description: "[head] must be the version directory name with the highest version number.",
			URL:         "https://ocfl.io/1.1/spec/#E040",
		},
	})

// E041: In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.
var E041 = validation.NewCode("E041",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.",
			URL:         "https://ocfl.io/1.0/spec/#E041",
		},
		ocfl.Spec1_1: {
			Description: "In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.",
			URL:         "https://ocfl.io/1.1/spec/#E041",
		},
	})

// E042: Content paths within a manifest block must be relative to the OCFL Object Root.
var E042 = validation.NewCode("E042",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Content paths within a manifest block must be relative to the OCFL Object Root.",
			URL:         "https://ocfl.io/1.0/spec/#E042",
		},
		ocfl.Spec1_1: {
			Description: "Content paths within a manifest block must be relative to the OCFL Object Root.",
			URL:         "https://ocfl.io/1.1/spec/#E042",
		},
	})

// E043: An OCFL Object Inventory must include a block for storing versions.
var E043 = validation.NewCode("E043",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "An OCFL Object Inventory must include a block for storing versions.",
			URL:         "https://ocfl.io/1.0/spec/#E043",
		},
		ocfl.Spec1_1: {
			Description: "An OCFL Object Inventory must include a block for storing versions.",
			URL:         "https://ocfl.io/1.1/spec/#E043",
		},
	})

// E044: This block MUST have the key of versions within the inventory, and it must be a JSON object.
var E044 = validation.NewCode("E044",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "This block MUST have the key of versions within the inventory, and it must be a JSON object.",
			URL:         "https://ocfl.io/1.0/spec/#E044",
		},
		ocfl.Spec1_1: {
			Description: "This block MUST have the key of versions within the inventory, and it must be a JSON object.",
			URL:         "https://ocfl.io/1.1/spec/#E044",
		},
	})

// E045: This block must have the key of versions within the inventory, and it MUST be a JSON object.
var E045 = validation.NewCode("E045",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "This block must have the key of versions within the inventory, and it MUST be a JSON object.",
			URL:         "https://ocfl.io/1.0/spec/#E045",
		},
		ocfl.Spec1_1: {
			Description: "This block must have the key of versions within the inventory, and it MUST be a JSON object.",
			URL:         "https://ocfl.io/1.1/spec/#E045",
		},
	})

// E046: The keys of [the versions object] must correspond to the names of the version directories used.
var E046 = validation.NewCode("E046",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The keys of [the versions object] must correspond to the names of the version directories used.",
			URL:         "https://ocfl.io/1.0/spec/#E046",
		},
		ocfl.Spec1_1: {
			Description: "The keys of [the versions object] must correspond to the names of the version directories used.",
			URL:         "https://ocfl.io/1.1/spec/#E046",
		},
	})

// E047: Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.
var E047 = validation.NewCode("E047",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.",
			URL:         "https://ocfl.io/1.0/spec/#E047",
		},
		ocfl.Spec1_1: {
			Description: "Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.",
			URL:         "https://ocfl.io/1.1/spec/#E047",
		},
	})

// E048: A JSON object to describe one OCFL Version, which must include the following keys: [created, state]
var E048 = validation.NewCode("E048",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "A JSON object to describe one OCFL Version, which must include the following keys: [created, state]",
			URL:         "https://ocfl.io/1.0/spec/#E048",
		},
		ocfl.Spec1_1: {
			Description: "A JSON object to describe one OCFL Version, which must include the following keys: [created, state]",
			URL:         "https://ocfl.io/1.1/spec/#E048",
		},
	})

// E049: [the value of the \"created\" key] must be expressed in the Internet Date/Time Format defined by [RFC3339].
var E049 = validation.NewCode("E049",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[the value of the \"created\" key] must be expressed in the Internet Date/Time Format defined by [RFC3339].",
			URL:         "https://ocfl.io/1.0/spec/#E049",
		},
		ocfl.Spec1_1: {
			Description: "[the value of the “created” key] must be expressed in the Internet Date/Time Format defined by [RFC3339].",
			URL:         "https://ocfl.io/1.1/spec/#E049",
		},
	})

// E050: The keys of [the \"state\" JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.
var E050 = validation.NewCode("E050",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The keys of [the \"state\" JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E050",
		},
		ocfl.Spec1_1: {
			Description: "The keys of [the “state” JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E050",
		},
	})

// E051: The logical path [value of a \"state\" digest key] must be interpreted as a set of one or more path elements joined by a / path separator.
var E051 = validation.NewCode("E051",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The logical path [value of a \"state\" digest key] must be interpreted as a set of one or more path elements joined by a / path separator.",
			URL:         "https://ocfl.io/1.0/spec/#E051",
		},
		ocfl.Spec1_1: {
			Description: "The logical path [value of a “state” digest key] must be interpreted as a set of one or more path elements joined by a / path separator.",
			URL:         "https://ocfl.io/1.1/spec/#E051",
		},
	})

// E052: [logical] Path elements must not be ., .., or empty (//).
var E052 = validation.NewCode("E052",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[logical] Path elements must not be ., .., or empty (//).",
			URL:         "https://ocfl.io/1.0/spec/#E052",
		},
		ocfl.Spec1_1: {
			Description: "[logical] Path elements must not be ., .., or empty (//).",
			URL:         "https://ocfl.io/1.1/spec/#E052",
		},
	})

// E053: Additionally, a logical path must not begin or end with a forward slash (/).
var E053 = validation.NewCode("E053",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Additionally, a logical path must not begin or end with a forward slash (/).",
			URL:         "https://ocfl.io/1.0/spec/#E053",
		},
		ocfl.Spec1_1: {
			Description: "Additionally, a logical path must not begin or end with a forward slash (/).",
			URL:         "https://ocfl.io/1.1/spec/#E053",
		},
	})

// E054: The value of the user key must contain a user name key, \"name\" and should contain an address key, \"address\".
var E054 = validation.NewCode("E054",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The value of the user key must contain a user name key, \"name\" and should contain an address key, \"address\".",
			URL:         "https://ocfl.io/1.0/spec/#E054",
		},
		ocfl.Spec1_1: {
			Description: "The value of the user key must contain a user name key, “name” and should contain an address key, “address”.",
			URL:         "https://ocfl.io/1.1/spec/#E054",
		},
	})

// E055: This block must have the key of fixity within the inventory.
var E055 = validation.NewCode("E055",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "This block must have the key of fixity within the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E055",
		},
		ocfl.Spec1_1: {
			Description: "If present, [the fixity] block must have the key of fixity within the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E055",
		},
	})

// E056: The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.
var E056 = validation.NewCode("E056",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.",
			URL:         "https://ocfl.io/1.0/spec/#E056",
		},
		ocfl.Spec1_1: {
			Description: "The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.",
			URL:         "https://ocfl.io/1.1/spec/#E056",
		},
	})

// E057: The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.
var E057 = validation.NewCode("E057",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.",
			URL:         "https://ocfl.io/1.0/spec/#E057",
		},
		ocfl.Spec1_1: {
			Description: "The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.",
			URL:         "https://ocfl.io/1.1/spec/#E057",
		},
	})

// E058: Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.
var E058 = validation.NewCode("E058",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.",
			URL:         "https://ocfl.io/1.0/spec/#E058",
		},
		ocfl.Spec1_1: {
			Description: "Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.",
			URL:         "https://ocfl.io/1.1/spec/#E058",
		},
	})

// E059: This value must match the value given for the digestAlgorithm key in the inventory.
var E059 = validation.NewCode("E059",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "This value must match the value given for the digestAlgorithm key in the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E059",
		},
		ocfl.Spec1_1: {
			Description: "This value must match the value given for the digestAlgorithm key in the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E059",
		},
	})

// E060: The digest sidecar file must contain the digest of the inventory file.
var E060 = validation.NewCode("E060",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The digest sidecar file must contain the digest of the inventory file.",
			URL:         "https://ocfl.io/1.0/spec/#E060",
		},
		ocfl.Spec1_1: {
			Description: "The digest sidecar file must contain the digest of the inventory file.",
			URL:         "https://ocfl.io/1.1/spec/#E060",
		},
	})

// E061: [The digest sidecar file] must follow the format: DIGEST inventory.json
var E061 = validation.NewCode("E061",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[The digest sidecar file] must follow the format: DIGEST inventory.json",
			URL:         "https://ocfl.io/1.0/spec/#E061",
		},
		ocfl.Spec1_1: {
			Description: "[The digest sidecar file] must follow the format: DIGEST inventory.json",
			URL:         "https://ocfl.io/1.1/spec/#E061",
		},
	})

// E062: The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.
var E062 = validation.NewCode("E062",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.",
			URL:         "https://ocfl.io/1.0/spec/#E062",
		},
		ocfl.Spec1_1: {
			Description: "The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.",
			URL:         "https://ocfl.io/1.1/spec/#E062",
		},
	})

// E063: Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.
var E063 = validation.NewCode("E063",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.",
			URL:         "https://ocfl.io/1.0/spec/#E063",
		},
		ocfl.Spec1_1: {
			Description: "Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.",
			URL:         "https://ocfl.io/1.1/spec/#E063",
		},
	})

// E064: Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.
var E064 = validation.NewCode("E064",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.",
			URL:         "https://ocfl.io/1.0/spec/#E064",
		},
		ocfl.Spec1_1: {
			Description: "Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.",
			URL:         "https://ocfl.io/1.1/spec/#E064",
		},
	})

// E066: Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.
var E066 = validation.NewCode("E066",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.",
			URL:         "https://ocfl.io/1.0/spec/#E066",
		},
		ocfl.Spec1_1: {
			Description: "Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.",
			URL:         "https://ocfl.io/1.1/spec/#E066",
		},
	})

// E067: The extensions directory must not contain any files, and no sub-directories other than extension sub-directories.
var E067 = validation.NewCode("E067",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The extensions directory must not contain any files, and no sub-directories other than extension sub-directories.",
			URL:         "https://ocfl.io/1.0/spec/#E067",
		},
		ocfl.Spec1_1: {
			Description: "The extensions directory must not contain any files or sub-directories other than extension sub-directories.",
			URL:         "https://ocfl.io/1.1/spec/#E067",
		},
	})

// E068: The specific structure and function of the extension, as well as a declaration of the registered extension name must be defined in one of the following locations: The OCFL Extensions repository OR The Storage Root, as a plain text document directly in the Storage Root.
var E068 = validation.NewCode("E068",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The specific structure and function of the extension, as well as a declaration of the registered extension name must be defined in one of the following locations: The OCFL Extensions repository OR The Storage Root, as a plain text document directly in the Storage Root.",
			URL:         "https://ocfl.io/1.0/spec/#E068",
		},
	})

// E069: An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.
var E069 = validation.NewCode("E069",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.",
			URL:         "https://ocfl.io/1.0/spec/#E069",
		},
		ocfl.Spec1_1: {
			Description: "An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.",
			URL:         "https://ocfl.io/1.1/spec/#E069",
		},
	})

// E070: If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [key, description]
var E070 = validation.NewCode("E070",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [key, description]",
			URL:         "https://ocfl.io/1.0/spec/#E070",
		},
		ocfl.Spec1_1: {
			Description: "If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [extension, description]",
			URL:         "https://ocfl.io/1.1/spec/#E070",
		},
	})

// E071: The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.
var E071 = validation.NewCode("E071",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.",
			URL:         "https://ocfl.io/1.0/spec/#E071",
		},
		ocfl.Spec1_1: {
			Description: "The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.",
			URL:         "https://ocfl.io/1.1/spec/#E071",
		},
	})

// E072: The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.
var E072 = validation.NewCode("E072",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.",
			URL:         "https://ocfl.io/1.0/spec/#E072",
		},
		ocfl.Spec1_1: {
			Description: "The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.",
			URL:         "https://ocfl.io/1.1/spec/#E072",
		},
	})

// E073: Empty directories MUST NOT appear under a storage root.
var E073 = validation.NewCode("E073",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Empty directories MUST NOT appear under a storage root.",
			URL:         "https://ocfl.io/1.0/spec/#E073",
		},
		ocfl.Spec1_1: {
			Description: "Empty directories MUST NOT appear under a storage root.",
			URL:         "https://ocfl.io/1.1/spec/#E073",
		},
	})

// E074: Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple \"buckets\" in an object store - each OCFL Storage Root MUST be independent.
var E074 = validation.NewCode("E074",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple \"buckets\" in an object store - each OCFL Storage Root MUST be independent.",
			URL:         "https://ocfl.io/1.0/spec/#E074",
		},
		ocfl.Spec1_1: {
			Description: "Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple “buckets” in an object store - each OCFL Storage Root MUST be independent.",
			URL:         "https://ocfl.io/1.1/spec/#E074",
		},
	})

// E075: The OCFL version declaration MUST be formatted according to the NAMASTE specification.
var E075 = validation.NewCode("E075",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The OCFL version declaration MUST be formatted according to the NAMASTE specification.",
			URL:         "https://ocfl.io/1.0/spec/#E075",
		},
		ocfl.Spec1_1: {
			Description: "The OCFL version declaration MUST be formatted according to the NAMASTE specification.",
			URL:         "https://ocfl.io/1.1/spec/#E075",
		},
	})

// E076: [The OCFL version declaration] MUST be a file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.
var E076 = validation.NewCode("E076",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[The OCFL version declaration] MUST be a file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.",
			URL:         "https://ocfl.io/1.0/spec/#E076",
		},
		ocfl.Spec1_1: {
			Description: "There must be exactly one version declaration file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.",
			URL:         "https://ocfl.io/1.1/spec/#E076",
		},
	})

// E077: [The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification ocfl.Number.
var E077 = validation.NewCode("E077",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E077",
		},
		ocfl.Spec1_1: {
			Description: "[The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E077",
		},
	})

// E078: [The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification ocfl.Number.
var E078 = validation.NewCode("E078",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E078",
		},
		ocfl.Spec1_1: {
			Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E078",
		},
	})

// E079: [The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification ocfl.Number.
var E079 = validation.NewCode("E079",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E079",
		},
		ocfl.Spec1_1: {
			Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E079",
		},
	})

// E080: The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).
var E080 = validation.NewCode("E080",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).",
			URL:         "https://ocfl.io/1.0/spec/#E080",
		},
		ocfl.Spec1_1: {
			Description: "The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).",
			URL:         "https://ocfl.io/1.1/spec/#E080",
		},
	})

// E081: OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.
var E081 = validation.NewCode("E081",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.",
			URL:         "https://ocfl.io/1.0/spec/#E081",
		},
		ocfl.Spec1_1: {
			Description: "OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.",
			URL:         "https://ocfl.io/1.1/spec/#E081",
		},
	})

// E082: OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.
var E082 = validation.NewCode("E082",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.",
			URL:         "https://ocfl.io/1.0/spec/#E082",
		},
		ocfl.Spec1_1: {
			Description: "OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.",
			URL:         "https://ocfl.io/1.1/spec/#E082",
		},
	})

// E083: There MUST be a deterministic mapping from an object identifier to a unique storage path.
var E083 = validation.NewCode("E083",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "There MUST be a deterministic mapping from an object identifier to a unique storage path.",
			URL:         "https://ocfl.io/1.0/spec/#E083",
		},
		ocfl.Spec1_1: {
			Description: "There MUST be a deterministic mapping from an object identifier to a unique storage path.",
			URL:         "https://ocfl.io/1.1/spec/#E083",
		},
	})

// E084: Storage hierarchies MUST NOT include files within intermediate directories.
var E084 = validation.NewCode("E084",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Storage hierarchies MUST NOT include files within intermediate directories.",
			URL:         "https://ocfl.io/1.0/spec/#E084",
		},
		ocfl.Spec1_1: {
			Description: "Storage hierarchies MUST NOT include files within intermediate directories.",
			URL:         "https://ocfl.io/1.1/spec/#E084",
		},
	})

// E085: Storage hierarchies MUST be terminated by OCFL Object Roots.
var E085 = validation.NewCode("E085",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Storage hierarchies MUST be terminated by OCFL Object Roots.",
			URL:         "https://ocfl.io/1.0/spec/#E085",
		},
		ocfl.Spec1_1: {
			Description: "Storage hierarchies MUST be terminated by OCFL Object Roots.",
			URL:         "https://ocfl.io/1.1/spec/#E085",
		},
	})

// E086: The storage root extensions directory MUST conform to the same guidelines and limitations as those defined for object extensions.
var E086 = validation.NewCode("E086",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The storage root extensions directory MUST conform to the same guidelines and limitations as those defined for object extensions.",
			URL:         "https://ocfl.io/1.0/spec/#E086",
		},
	})

// E087: An OCFL validator MUST ignore any files in the storage root it does not understand.
var E087 = validation.NewCode("E087",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "An OCFL validator MUST ignore any files in the storage root it does not understand.",
			URL:         "https://ocfl.io/1.0/spec/#E087",
		},
		ocfl.Spec1_1: {
			Description: "An OCFL validator MUST ignore any files in the storage root it does not understand.",
			URL:         "https://ocfl.io/1.1/spec/#E087",
		},
	})

// E088: An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.
var E088 = validation.NewCode("E088",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.",
			URL:         "https://ocfl.io/1.0/spec/#E088",
		},
		ocfl.Spec1_1: {
			Description: "An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.",
			URL:         "https://ocfl.io/1.1/spec/#E088",
		},
	})

// E089: If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.
var E089 = validation.NewCode("E089",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.",
			URL:         "https://ocfl.io/1.0/spec/#E089",
		},
		ocfl.Spec1_1: {
			Description: "If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.",
			URL:         "https://ocfl.io/1.1/spec/#E089",
		},
	})

// E090: Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierarchies.
var E090 = validation.NewCode("E090",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierarchies.",
			URL:         "https://ocfl.io/1.0/spec/#E090",
		},
		ocfl.Spec1_1: {
			Description: "Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierarchies.",
			URL:         "https://ocfl.io/1.1/spec/#E090",
		},
	})

// E091: Filesystems MUST preserve the case of OCFL filepaths and filenames.
var E091 = validation.NewCode("E091",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Filesystems MUST preserve the case of OCFL filepaths and filenames.",
			URL:         "https://ocfl.io/1.0/spec/#E091",
		},
		ocfl.Spec1_1: {
			Description: "Filesystems MUST preserve the case of OCFL filepaths and filenames.",
			URL:         "https://ocfl.io/1.1/spec/#E091",
		},
	})

// E092: The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.
var E092 = validation.NewCode("E092",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.",
			URL:         "https://ocfl.io/1.0/spec/#E092",
		},
		ocfl.Spec1_1: {
			Description: "The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.",
			URL:         "https://ocfl.io/1.1/spec/#E092",
		},
	})

// E093: Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.
var E093 = validation.NewCode("E093",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.",
			URL:         "https://ocfl.io/1.0/spec/#E093",
		},
		ocfl.Spec1_1: {
			Description: "Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.",
			URL:         "https://ocfl.io/1.1/spec/#E093",
		},
	})

// E094: The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.
var E094 = validation.NewCode("E094",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.",
			URL:         "https://ocfl.io/1.0/spec/#E094",
		},
		ocfl.Spec1_1: {
			Description: "The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.",
			URL:         "https://ocfl.io/1.1/spec/#E094",
		},
	})

// E095: Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.
var E095 = validation.NewCode("E095",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.",
			URL:         "https://ocfl.io/1.0/spec/#E095",
		},
		ocfl.Spec1_1: {
			Description: "Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.",
			URL:         "https://ocfl.io/1.1/spec/#E095",
		},
	})

// E096: As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.
var E096 = validation.NewCode("E096",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.",
			URL:         "https://ocfl.io/1.0/spec/#E096",
		},
		ocfl.Spec1_1: {
			Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.",
			URL:         "https://ocfl.io/1.1/spec/#E096",
		},
	})

// E097: As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.
var E097 = validation.NewCode("E097",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.",
			URL:         "https://ocfl.io/1.0/spec/#E097",
		},
		ocfl.Spec1_1: {
			Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.",
			URL:         "https://ocfl.io/1.1/spec/#E097",
		},
	})

// E098: The content path must be interpreted as a set of one or more path elements joined by a / path separator.
var E098 = validation.NewCode("E098",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The content path must be interpreted as a set of one or more path elements joined by a / path separator.",
			URL:         "https://ocfl.io/1.0/spec/#E098",
		},
		ocfl.Spec1_1: {
			Description: "The content path must be interpreted as a set of one or more path elements joined by a / path separator.",
			URL:         "https://ocfl.io/1.1/spec/#E098",
		},
	})

// E099: [content] path elements must not be ., .., or empty (//).
var E099 = validation.NewCode("E099",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "[content] path elements must not be ., .., or empty (//).",
			URL:         "https://ocfl.io/1.0/spec/#E099",
		},
		ocfl.Spec1_1: {
			Description: "[content] path elements must not be ., .., or empty (//).",
			URL:         "https://ocfl.io/1.1/spec/#E099",
		},
	})

// E100: A content path must not begin or end with a forward slash (/).
var E100 = validation.NewCode("E100",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "A content path must not begin or end with a forward slash (/).",
			URL:         "https://ocfl.io/1.0/spec/#E100",
		},
		ocfl.Spec1_1: {
			Description: "A content path must not begin or end with a forward slash (/).",
			URL:         "https://ocfl.io/1.1/spec/#E100",
		},
	})

// E101: Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.
var E101 = validation.NewCode("E101",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.",
			URL:         "https://ocfl.io/1.0/spec/#E101",
		},
		ocfl.Spec1_1: {
			Description: "Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.",
			URL:         "https://ocfl.io/1.1/spec/#E101",
		},
	})

// E102: An inventory file must not contain keys that are not specified.
var E102 = validation.NewCode("E102",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "An inventory file must not contain keys that are not specified.",
			URL:         "https://ocfl.io/1.0/spec/#E102",
		},
		ocfl.Spec1_1: {
			Description: "An inventory file must not contain keys that are not specified.",
			URL:         "https://ocfl.io/1.1/spec/#E102",
		},
	})

// E103: Each version directory within an OCFL Object MUST conform to either the same or a later OCFL specification version as the preceding version directory.
var E103 = validation.NewCode("E103",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "Each version directory within an OCFL Object MUST conform to either the same or a later OCFL specification version as the preceding version directory.",
			URL:         "https://ocfl.io/1.1/spec/#E103",
		},
	})

// E104: Version directory names MUST be constructed by prepending v to the version number.
var E104 = validation.NewCode("E104",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "Version directory names MUST be constructed by prepending v to the version number.",
			URL:         "https://ocfl.io/1.1/spec/#E104",
		},
	})

// E105: The version number MUST be taken from the sequence of positive, base-ten integers: 1, 2, 3, etc.
var E105 = validation.NewCode("E105",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "The version number MUST be taken from the sequence of positive, base-ten integers: 1, 2, 3, etc.",
			URL:         "https://ocfl.io/1.1/spec/#E105",
		},
	})

// E106: The value of the manifest key MUST be a JSON object.
var E106 = validation.NewCode("E106",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "The value of the manifest key MUST be a JSON object.",
			URL:         "https://ocfl.io/1.1/spec/#E106",
		},
	})

// E107: The value of the manifest key must be a JSON object, and each key MUST correspond to a digest value key found in one or more state blocks of the current and/or previous version blocks of the OCFL Object.
var E107 = validation.NewCode("E107",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "The value of the manifest key must be a JSON object, and each key MUST correspond to a digest value key found in one or more state blocks of the current and/or previous version blocks of the OCFL Object.",
			URL:         "https://ocfl.io/1.1/spec/#E107",
		},
	})

// E108: The contentDirectory value MUST represent a direct child directory of the version directory in which it is found.
var E108 = validation.NewCode("E108",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "The contentDirectory value MUST represent a direct child directory of the version directory in which it is found.",
			URL:         "https://ocfl.io/1.1/spec/#E108",
		},
	})

// E110: A unique identifier for the OCFL Object MUST NOT change between versions of the same object.
var E110 = validation.NewCode("E110",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "A unique identifier for the OCFL Object MUST NOT change between versions of the same object.",
			URL:         "https://ocfl.io/1.1/spec/#E110",
		},
	})

// E111: If present, [the value of the fixity key] MUST be a JSON object, which may be empty.
var E111 = validation.NewCode("E111",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "If present, [the value of the fixity key] MUST be a JSON object, which may be empty.",
			URL:         "https://ocfl.io/1.1/spec/#E111",
		},
	})

// E112: The extensions directory must not contain any files or sub-directories other than extension sub-directories.
var E112 = validation.NewCode("E112",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "The extensions directory must not contain any files or sub-directories other than extension sub-directories.",
			URL:         "https://ocfl.io/1.1/spec/#E112",
		},
	})

// W001: Implementations SHOULD use version directory names constructed without zero-padding the ocfl.Number, ie. v1, v2, v3, etc.
var W001 = validation.NewCode("W001",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Implementations SHOULD use version directory names constructed without zero-padding the ocfl.Number, ie. v1, v2, v3, etc.",
			URL:         "https://ocfl.io/1.0/spec/#W001",
		},
		ocfl.Spec1_1: {
			Description: "Implementations SHOULD use version directory names constructed without zero-padding the version number, ie. v1, v2, v3, etc.",
			URL:         "https://ocfl.io/1.1/spec/#W001",
		},
	})

// W002: The version directory SHOULD NOT contain any directories other than the designated content sub-directory. Once created, the contents of a version directory are expected to be immutable.
var W002 = validation.NewCode("W002",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The version directory SHOULD NOT contain any directories other than the designated content sub-directory. Once created, the contents of a version directory are expected to be immutable.",
			URL:         "https://ocfl.io/1.0/spec/#W002",
		},
		ocfl.Spec1_1: {
			Description: "The version directory SHOULD NOT contain any directories other than the designated content sub-directory. Once created, the contents of a version directory are expected to be immutable.",
			URL:         "https://ocfl.io/1.1/spec/#W002",
		},
	})

// W003: Version directories must contain a designated content sub-directory if the version contains files to be preserved, and SHOULD NOT contain this sub-directory otherwise.
var W003 = validation.NewCode("W003",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and SHOULD NOT contain this sub-directory otherwise.",
			URL:         "https://ocfl.io/1.0/spec/#W003",
		},
		ocfl.Spec1_1: {
			Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and SHOULD NOT contain this sub-directory otherwise.",
			URL:         "https://ocfl.io/1.1/spec/#W003",
		},
	})

// W004: For content-addressing, OCFL Objects SHOULD use sha512.
var W004 = validation.NewCode("W004",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "For content-addressing, OCFL Objects SHOULD use sha512.",
			URL:         "https://ocfl.io/1.0/spec/#W004",
		},
		ocfl.Spec1_1: {
			Description: "For content-addressing, OCFL Objects SHOULD use sha512.",
			URL:         "https://ocfl.io/1.1/spec/#W004",
		},
	})

// W005: The OCFL Object Inventory id SHOULD be a URI.
var W005 = validation.NewCode("W005",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "The OCFL Object Inventory id SHOULD be a URI.",
			URL:         "https://ocfl.io/1.0/spec/#W005",
		},
		ocfl.Spec1_1: {
			Description: "The OCFL Object Inventory id SHOULD be a URI.",
			URL:         "https://ocfl.io/1.1/spec/#W005",
		},
	})

// W007: In the OCFL Object Inventory, the JSON object describing an OCFL Version, SHOULD include the message and user keys.
var W007 = validation.NewCode("W007",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In the OCFL Object Inventory, the JSON object describing an OCFL Version, SHOULD include the message and user keys.",
			URL:         "https://ocfl.io/1.0/spec/#W007",
		},
		ocfl.Spec1_1: {
			Description: "In the OCFL Object Inventory, the JSON object describing an OCFL Version, SHOULD include the message and user keys.",
			URL:         "https://ocfl.io/1.1/spec/#W007",
		},
	})

// W008: In the OCFL Object Inventory, in the version block, the value of the user key SHOULD contain an address key, address.
var W008 = validation.NewCode("W008",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In the OCFL Object Inventory, in the version block, the value of the user key SHOULD contain an address key, address.",
			URL:         "https://ocfl.io/1.0/spec/#W008",
		},
		ocfl.Spec1_1: {
			Description: "In the OCFL Object Inventory, in the version block, the value of the user key SHOULD contain an address key, address.",
			URL:         "https://ocfl.io/1.1/spec/#W008",
		},
	})

// W009: In the OCFL Object Inventory, in the version block, the address value SHOULD be a URI: either a mailto URI [RFC6068] with the e-mail address of the user or a URL to a personal identifier, e.g., an ORCID iD.
var W009 = validation.NewCode("W009",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In the OCFL Object Inventory, in the version block, the address value SHOULD be a URI: either a mailto URI [RFC6068] with the e-mail address of the user or a URL to a personal identifier, e.g., an ORCID iD.",
			URL:         "https://ocfl.io/1.0/spec/#W009",
		},
		ocfl.Spec1_1: {
			Description: "In the OCFL Object Inventory, in the version block, the address value SHOULD be a URI: either a mailto URI [RFC6068] with the e-mail address of the user or a URL to a personal identifier, e.g., an ORCID iD.",
			URL:         "https://ocfl.io/1.1/spec/#W009",
		},
	})

// W010: In addition to the inventory in the OCFL Object Root, every version directory SHOULD include an inventory file that is an Inventory of all content for versions up to and including that particular version.
var W010 = validation.NewCode("W010",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In addition to the inventory in the OCFL Object Root, every version directory SHOULD include an inventory file that is an Inventory of all content for versions up to and including that particular version.",
			URL:         "https://ocfl.io/1.0/spec/#W010",
		},
		ocfl.Spec1_1: {
			Description: "In addition to the inventory in the OCFL Object Root, every version directory SHOULD include an inventory file that is an Inventory of all content for versions up to and including that particular version.",
			URL:         "https://ocfl.io/1.1/spec/#W010",
		},
	})

// W011: In the case that prior version directories include an inventory file, the values of the created, message and user keys in each version block in each prior inventory file SHOULD have the same values as the corresponding keys in the corresponding version block in the current inventory file.
var W011 = validation.NewCode("W011",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In the case that prior version directories include an inventory file, the values of the created, message and user keys in each version block in each prior inventory file SHOULD have the same values as the corresponding keys in the corresponding version block in the current inventory file.",
			URL:         "https://ocfl.io/1.0/spec/#W011",
		},
		ocfl.Spec1_1: {
			Description: "In the case that prior version directories include an inventory file, the values of the created, message and user keys in each version block in each prior inventory file SHOULD have the same values as the corresponding keys in the corresponding version block in the current inventory file.",
			URL:         "https://ocfl.io/1.1/spec/#W011",
		},
	})

// W012: Implementers SHOULD use the logs directory, if present, for storing files that contain a record of actions taken on the object.
var W012 = validation.NewCode("W012",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Implementers SHOULD use the logs directory, if present, for storing files that contain a record of actions taken on the object.",
			URL:         "https://ocfl.io/1.0/spec/#W012",
		},
		ocfl.Spec1_1: {
			Description: "Implementers SHOULD use the logs directory, if present, for storing files that contain a record of actions taken on the object.",
			URL:         "https://ocfl.io/1.1/spec/#W012",
		},
	})

// W013: In an OCFL Object, extension sub-directories SHOULD be named according to a registered extension name.
var W013 = validation.NewCode("W013",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "In an OCFL Object, extension sub-directories SHOULD be named according to a registered extension name.",
			URL:         "https://ocfl.io/1.0/spec/#W013",
		},
		ocfl.Spec1_1: {
			Description: "In an OCFL Object, extension sub-directories SHOULD be named according to a registered extension name.",
			URL:         "https://ocfl.io/1.1/spec/#W013",
		},
	})

// W014: Storage hierarchies within the same OCFL Storage Root SHOULD use just one layout pattern.
var W014 = validation.NewCode("W014",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Storage hierarchies within the same OCFL Storage Root SHOULD use just one layout pattern.",
			URL:         "https://ocfl.io/1.0/spec/#W014",
		},
		ocfl.Spec1_1: {
			Description: "Storage hierarchies within the same OCFL Storage Root SHOULD use just one layout pattern.",
			URL:         "https://ocfl.io/1.1/spec/#W014",
		},
	})

// W015: Storage hierarchies within the same OCFL Storage Root SHOULD consistently use either a directory hierarchy of OCFL Objects or top-level OCFL Objects.
var W015 = validation.NewCode("W015",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_0: {
			Description: "Storage hierarchies within the same OCFL Storage Root SHOULD consistently use either a directory hierarchy of OCFL Objects or top-level OCFL Objects.",
			URL:         "https://ocfl.io/1.0/spec/#W015",
		},
		ocfl.Spec1_1: {
			Description: "Storage hierarchies within the same OCFL Storage Root SHOULD consistently use either a directory hierarchy of OCFL Objects or top-level OCFL Objects.",
			URL:         "https://ocfl.io/1.1/spec/#W015",
		},
	})

// W016: In the Storage Root, extension sub-directories SHOULD be named according to a registered extension name.
var W016 = validation.NewCode("W016",
	map[ocfl.Spec]*validation.Ref{
		ocfl.Spec1_1: {
			Description: "In the Storage Root, extension sub-directories SHOULD be named according to a registered extension name.",
			URL:         "https://ocfl.io/1.1/spec/#W016",
		},
	})
