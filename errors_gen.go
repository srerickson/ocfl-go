package ocfl

// This is generated code. Do not modify. See errors_gen folder.


var ErrE001 = ObjectValidationErr{
	Description: "The OCFL Object Root must not contain files or directories other than those specified in the following sections.",
	Code: "E001",
	URI: "https://ocfl.io/1.0/spec/#E001",
}


var ErrE002 = ObjectValidationErr{
	Description: "The version declaration must be formatted according to the NAMASTE specification.",
	Code: "E002",
	URI: "https://ocfl.io/1.0/spec/#E002",
}


var ErrE003 = ObjectValidationErr{
	Description: "[The version declaration] must be a file in the base directory of the OCFL Object Root giving the OCFL version in the filename.",
	Code: "E003",
	URI: "https://ocfl.io/1.0/spec/#E003",
}


var ErrE004 = ObjectValidationErr{
	Description: "The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.",
	Code: "E004",
	URI: "https://ocfl.io/1.0/spec/#E004",
}


var ErrE005 = ObjectValidationErr{
	Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.",
	Code: "E005",
	URI: "https://ocfl.io/1.0/spec/#E005",
}


var ErrE006 = ObjectValidationErr{
	Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification version number.",
	Code: "E006",
	URI: "https://ocfl.io/1.0/spec/#E006",
}


var ErrE007 = ObjectValidationErr{
	Description: "The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).",
	Code: "E007",
	URI: "https://ocfl.io/1.0/spec/#E007",
}


var ErrE008 = ObjectValidationErr{
	Description: "OCFL Object content must be stored as a sequence of one or more versions.",
	Code: "E008",
	URI: "https://ocfl.io/1.0/spec/#E008",
}


var ErrE009 = ObjectValidationErr{
	Description: "The version number sequence MUST start at 1 and must be continuous without missing integers.",
	Code: "E009",
	URI: "https://ocfl.io/1.0/spec/#E009",
}


var ErrE010 = ObjectValidationErr{
	Description: "The version number sequence must start at 1 and MUST be continuous without missing integers.",
	Code: "E010",
	URI: "https://ocfl.io/1.0/spec/#E010",
}


var ErrE011 = ObjectValidationErr{
	Description: "If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.",
	Code: "E011",
	URI: "https://ocfl.io/1.0/spec/#E011",
}


var ErrE012 = ObjectValidationErr{
	Description: "All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.",
	Code: "E012",
	URI: "https://ocfl.io/1.0/spec/#E012",
}


var ErrE013 = ObjectValidationErr{
	Description: "Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.",
	Code: "E013",
	URI: "https://ocfl.io/1.0/spec/#E013",
}


var ErrE014 = ObjectValidationErr{
	Description: "In all cases, references to files inside version directories from inventory files must use the actual version directory names.",
	Code: "E014",
	URI: "https://ocfl.io/1.0/spec/#E014",
}


var ErrE015 = ObjectValidationErr{
	Description: "There must be no other files as children of a version directory, other than an inventory file and a inventory digest.",
	Code: "E015",
	URI: "https://ocfl.io/1.0/spec/#E015",
}


var ErrE016 = ObjectValidationErr{
	Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.",
	Code: "E016",
	URI: "https://ocfl.io/1.0/spec/#E016",
}


var ErrE017 = ObjectValidationErr{
	Description: "The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).",
	Code: "E017",
	URI: "https://ocfl.io/1.0/spec/#E017",
}


var ErrE018 = ObjectValidationErr{
	Description: "The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).",
	Code: "E018",
	URI: "https://ocfl.io/1.0/spec/#E018",
}


var ErrE019 = ObjectValidationErr{
	Description: "If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.",
	Code: "E019",
	URI: "https://ocfl.io/1.0/spec/#E019",
}


var ErrE020 = ObjectValidationErr{
	Description: "If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.",
	Code: "E020",
	URI: "https://ocfl.io/1.0/spec/#E020",
}


var ErrE021 = ObjectValidationErr{
	Description: "If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.",
	Code: "E021",
	URI: "https://ocfl.io/1.0/spec/#E021",
}


var ErrE022 = ObjectValidationErr{
	Description: "OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.",
	Code: "E022",
	URI: "https://ocfl.io/1.0/spec/#E022",
}


var ErrE023 = ObjectValidationErr{
	Description: "Every file within a version's content directory must be referenced in the manifest section of the inventory.",
	Code: "E023",
	URI: "https://ocfl.io/1.0/spec/#E023",
}


var ErrE024 = ObjectValidationErr{
	Description: "There must not be empty directories within a version's content directory.",
	Code: "E024",
	URI: "https://ocfl.io/1.0/spec/#E024",
}


var ErrE025 = ObjectValidationErr{
	Description: "For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.",
	Code: "E025",
	URI: "https://ocfl.io/1.0/spec/#E025",
}


var ErrE026 = ObjectValidationErr{
	Description: "For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].",
	Code: "E026",
	URI: "https://ocfl.io/1.0/spec/#E026",
}


var ErrE027 = ObjectValidationErr{
	Description: "OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.",
	Code: "E027",
	URI: "https://ocfl.io/1.0/spec/#E027",
}


var ErrE028 = ObjectValidationErr{
	Description: "Optional fixity algorithms that are not supported by a client must be ignored by that client.",
	Code: "E028",
	URI: "https://ocfl.io/1.0/spec/#E028",
}


var ErrE029 = ObjectValidationErr{
	Description: "SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
	Code: "E029",
	URI: "https://ocfl.io/1.0/spec/#E029",
}


var ErrE030 = ObjectValidationErr{
	Description: "SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
	Code: "E030",
	URI: "https://ocfl.io/1.0/spec/#E030",
}


var ErrE031 = ObjectValidationErr{
	Description: "SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
	Code: "E031",
	URI: "https://ocfl.io/1.0/spec/#E031",
}


var ErrE032 = ObjectValidationErr{
	Description: "[blake2b-512] must be encoded using hex (base16) encoding [RFC4648].",
	Code: "E032",
	URI: "https://ocfl.io/1.0/spec/#E032",
}


var ErrE033 = ObjectValidationErr{
	Description: "An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.",
	Code: "E033",
	URI: "https://ocfl.io/1.0/spec/#E033",
}


var ErrE034 = ObjectValidationErr{
	Description: "An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.",
	Code: "E034",
	URI: "https://ocfl.io/1.0/spec/#E034",
}


var ErrE035 = ObjectValidationErr{
	Description: "The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.",
	Code: "E035",
	URI: "https://ocfl.io/1.0/spec/#E035",
}


var ErrE036 = ObjectValidationErr{
	Description: "An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]",
	Code: "E036",
	URI: "https://ocfl.io/1.0/spec/#E036",
}


var ErrE037 = ObjectValidationErr{
	Description: "[id] must be unique in the local context, and should be a URI [RFC3986].",
	Code: "E037",
	URI: "https://ocfl.io/1.0/spec/#E037",
}


var ErrE038 = ObjectValidationErr{
	Description: "In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.",
	Code: "E038",
	URI: "https://ocfl.io/1.0/spec/#E038",
}


var ErrE039 = ObjectValidationErr{
	Description: "[digestAlgorithm] must be the algorithm used in the manifest and state blocks.",
	Code: "E039",
	URI: "https://ocfl.io/1.0/spec/#E039",
}


var ErrE040 = ObjectValidationErr{
	Description: "[head] must be the version directory name with the highest version number.",
	Code: "E040",
	URI: "https://ocfl.io/1.0/spec/#E040",
}


var ErrE041 = ObjectValidationErr{
	Description: "In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.",
	Code: "E041",
	URI: "https://ocfl.io/1.0/spec/#E041",
}


var ErrE042 = ObjectValidationErr{
	Description: "Content paths within a manifest block must be relative to the OCFL Object Root.",
	Code: "E042",
	URI: "https://ocfl.io/1.0/spec/#E042",
}


var ErrE043 = ObjectValidationErr{
	Description: "An OCFL Object Inventory must include a block for storing versions.",
	Code: "E043",
	URI: "https://ocfl.io/1.0/spec/#E043",
}


var ErrE044 = ObjectValidationErr{
	Description: "This block MUST have the key of versions within the inventory, and it must be a JSON object.",
	Code: "E044",
	URI: "https://ocfl.io/1.0/spec/#E044",
}


var ErrE045 = ObjectValidationErr{
	Description: "This block must have the key of versions within the inventory, and it MUST be a JSON object.",
	Code: "E045",
	URI: "https://ocfl.io/1.0/spec/#E045",
}


var ErrE046 = ObjectValidationErr{
	Description: "The keys of [the versions object] must correspond to the names of the version directories used.",
	Code: "E046",
	URI: "https://ocfl.io/1.0/spec/#E046",
}


var ErrE047 = ObjectValidationErr{
	Description: "Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.",
	Code: "E047",
	URI: "https://ocfl.io/1.0/spec/#E047",
}


var ErrE048 = ObjectValidationErr{
	Description: "A JSON object to describe one OCFL Version, which must include the following keys: [created, state, message, user]",
	Code: "E048",
	URI: "https://ocfl.io/1.0/spec/#E048",
}


var ErrE049 = ObjectValidationErr{
	Description: "[the value of the “created” key] must be expressed in the Internet Date/Time Format defined by [RFC3339].",
	Code: "E049",
	URI: "https://ocfl.io/1.0/spec/#E049",
}


var ErrE050 = ObjectValidationErr{
	Description: "The keys of [the “state” JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.",
	Code: "E050",
	URI: "https://ocfl.io/1.0/spec/#E050",
}


var ErrE051 = ObjectValidationErr{
	Description: "The logical path [value of a “state” digest key] must be interpreted as a set of one or more path elements joined by a / path separator.",
	Code: "E051",
	URI: "https://ocfl.io/1.0/spec/#E051",
}


var ErrE052 = ObjectValidationErr{
	Description: "[logical] Path elements must not be ., .., or empty (//).",
	Code: "E052",
	URI: "https://ocfl.io/1.0/spec/#E052",
}


var ErrE053 = ObjectValidationErr{
	Description: "Additionally, a logical path must not begin or end with a forward slash (/).",
	Code: "E053",
	URI: "https://ocfl.io/1.0/spec/#E053",
}


var ErrE054 = ObjectValidationErr{
	Description: "The value of the user key must contain a user name key, “name” and should contain an address key, “address”.",
	Code: "E054",
	URI: "https://ocfl.io/1.0/spec/#E054",
}


var ErrE055 = ObjectValidationErr{
	Description: "This block must have the key of fixity within the inventory.",
	Code: "E055",
	URI: "https://ocfl.io/1.0/spec/#E055",
}


var ErrE056 = ObjectValidationErr{
	Description: "The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.",
	Code: "E056",
	URI: "https://ocfl.io/1.0/spec/#E056",
}


var ErrE057 = ObjectValidationErr{
	Description: "The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.",
	Code: "E057",
	URI: "https://ocfl.io/1.0/spec/#E057",
}


var ErrE058 = ObjectValidationErr{
	Description: "Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.",
	Code: "E058",
	URI: "https://ocfl.io/1.0/spec/#E058",
}


var ErrE059 = ObjectValidationErr{
	Description: "This value must match the value given for the digestAlgorithm key in the inventory.",
	Code: "E059",
	URI: "https://ocfl.io/1.0/spec/#E059",
}


var ErrE060 = ObjectValidationErr{
	Description: "The digest sidecar file must contain the digest of the inventory file.",
	Code: "E060",
	URI: "https://ocfl.io/1.0/spec/#E060",
}


var ErrE061 = ObjectValidationErr{
	Description: "[The digest sidecar file] must follow the format: DIGEST inventory.json",
	Code: "E061",
	URI: "https://ocfl.io/1.0/spec/#E061",
}


var ErrE062 = ObjectValidationErr{
	Description: "The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.",
	Code: "E062",
	URI: "https://ocfl.io/1.0/spec/#E062",
}


var ErrE063 = ObjectValidationErr{
	Description: "Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.",
	Code: "E063",
	URI: "https://ocfl.io/1.0/spec/#E063",
}


var ErrE064 = ObjectValidationErr{
	Description: "Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.",
	Code: "E064",
	URI: "https://ocfl.io/1.0/spec/#E064",
}


var ErrE066 = ObjectValidationErr{
	Description: "Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.",
	Code: "E066",
	URI: "https://ocfl.io/1.0/spec/#E066",
}


var ErrE067 = ObjectValidationErr{
	Description: "The extensions directory must not contain any files, and no sub-directories other than extension sub-directories.",
	Code: "E067",
	URI: "https://ocfl.io/1.0/spec/#E067",
}


var ErrE068 = ObjectValidationErr{
	Description: "The specific structure and function of the extension, as well as a declaration of the registered extension name must be defined in one of the following locations: The OCFL Extensions repository OR The Storage Root, as a plain text document directly in the Storage Root.",
	Code: "E068",
	URI: "https://ocfl.io/1.0/spec/#E068",
}


var ErrE069 = ObjectValidationErr{
	Description: "An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.",
	Code: "E069",
	URI: "https://ocfl.io/1.0/spec/#E069",
}


var ErrE070 = ObjectValidationErr{
	Description: "If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [key, description]",
	Code: "E070",
	URI: "https://ocfl.io/1.0/spec/#E070",
}


var ErrE071 = ObjectValidationErr{
	Description: "The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.",
	Code: "E071",
	URI: "https://ocfl.io/1.0/spec/#E071",
}


var ErrE072 = ObjectValidationErr{
	Description: "The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.",
	Code: "E072",
	URI: "https://ocfl.io/1.0/spec/#E072",
}


var ErrE073 = ObjectValidationErr{
	Description: "Empty directories MUST NOT appear under a storage root.",
	Code: "E073",
	URI: "https://ocfl.io/1.0/spec/#E073",
}


var ErrE074 = ObjectValidationErr{
	Description: "Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple “buckets” in an object store - each OCFL Storage Root MUST be independent.",
	Code: "E074",
	URI: "https://ocfl.io/1.0/spec/#E074",
}


var ErrE075 = ObjectValidationErr{
	Description: "The OCFL version declaration MUST be formatted according to the NAMASTE specification.",
	Code: "E075",
	URI: "https://ocfl.io/1.0/spec/#E075",
}


var ErrE076 = ObjectValidationErr{
	Description: "[The OCFL version declaration] MUST be a file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.",
	Code: "E076",
	URI: "https://ocfl.io/1.0/spec/#E076",
}


var ErrE077 = ObjectValidationErr{
	Description: "[The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.",
	Code: "E077",
	URI: "https://ocfl.io/1.0/spec/#E077",
}


var ErrE078 = ObjectValidationErr{
	Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.",
	Code: "E078",
	URI: "https://ocfl.io/1.0/spec/#E078",
}


var ErrE079 = ObjectValidationErr{
	Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification version number.",
	Code: "E079",
	URI: "https://ocfl.io/1.0/spec/#E079",
}


var ErrE080 = ObjectValidationErr{
	Description: "The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).",
	Code: "E080",
	URI: "https://ocfl.io/1.0/spec/#E080",
}


var ErrE081 = ObjectValidationErr{
	Description: "OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.",
	Code: "E081",
	URI: "https://ocfl.io/1.0/spec/#E081",
}


var ErrE082 = ObjectValidationErr{
	Description: "OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.",
	Code: "E082",
	URI: "https://ocfl.io/1.0/spec/#E082",
}


var ErrE083 = ObjectValidationErr{
	Description: "There MUST be a deterministic mapping from an object identifier to a unique storage path.",
	Code: "E083",
	URI: "https://ocfl.io/1.0/spec/#E083",
}


var ErrE084 = ObjectValidationErr{
	Description: "Storage hierarchies MUST NOT include files within intermediate directories.",
	Code: "E084",
	URI: "https://ocfl.io/1.0/spec/#E084",
}


var ErrE085 = ObjectValidationErr{
	Description: "Storage hierarchies MUST be terminated by OCFL Object Roots.",
	Code: "E085",
	URI: "https://ocfl.io/1.0/spec/#E085",
}


var ErrE086 = ObjectValidationErr{
	Description: "The storage root extensions directory MUST conform to the same guidelines and limitations as those defined for object extensions.",
	Code: "E086",
	URI: "https://ocfl.io/1.0/spec/#E086",
}


var ErrE087 = ObjectValidationErr{
	Description: "An OCFL validator MUST ignore any files in the storage root it does not understand.",
	Code: "E087",
	URI: "https://ocfl.io/1.0/spec/#E087",
}


var ErrE088 = ObjectValidationErr{
	Description: "An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.",
	Code: "E088",
	URI: "https://ocfl.io/1.0/spec/#E088",
}


var ErrE089 = ObjectValidationErr{
	Description: "If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.",
	Code: "E089",
	URI: "https://ocfl.io/1.0/spec/#E089",
}


var ErrE090 = ObjectValidationErr{
	Description: "Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierachies.",
	Code: "E090",
	URI: "https://ocfl.io/1.0/spec/#E090",
}


var ErrE091 = ObjectValidationErr{
	Description: "Filesystems MUST preserve the case of OCFL filepaths and filenames.",
	Code: "E091",
	URI: "https://ocfl.io/1.0/spec/#E091",
}


var ErrE092 = ObjectValidationErr{
	Description: "The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.",
	Code: "E092",
	URI: "https://ocfl.io/1.0/spec/#E092",
}


var ErrE093 = ObjectValidationErr{
	Description: "Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.",
	Code: "E093",
	URI: "https://ocfl.io/1.0/spec/#E093",
}


var ErrE094 = ObjectValidationErr{
	Description: "The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.",
	Code: "E094",
	URI: "https://ocfl.io/1.0/spec/#E094",
}


var ErrE095 = ObjectValidationErr{
	Description: "Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.",
	Code: "E095",
	URI: "https://ocfl.io/1.0/spec/#E095",
}


var ErrE096 = ObjectValidationErr{
	Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.",
	Code: "E096",
	URI: "https://ocfl.io/1.0/spec/#E096",
}


var ErrE097 = ObjectValidationErr{
	Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.",
	Code: "E097",
	URI: "https://ocfl.io/1.0/spec/#E097",
}


var ErrE098 = ObjectValidationErr{
	Description: "The content path must be interpreted as a set of one or more path elements joined by a / path separator.",
	Code: "E098",
	URI: "https://ocfl.io/1.0/spec/#E098",
}


var ErrE099 = ObjectValidationErr{
	Description: "[content] path elements must not be ., .., or empty (//).",
	Code: "E099",
	URI: "https://ocfl.io/1.0/spec/#E099",
}


var ErrE100 = ObjectValidationErr{
	Description: "A content path must not begin or end with a forward slash (/).",
	Code: "E100",
	URI: "https://ocfl.io/1.0/spec/#E100",
}


var ErrE101 = ObjectValidationErr{
	Description: "Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.",
	Code: "E101",
	URI: "https://ocfl.io/1.0/spec/#E101",
}


var ErrE102 = ObjectValidationErr{
	Description: "An inventory file must not contain keys that are not specified.",
	Code: "E102",
	URI: "https://ocfl.io/1.0/spec/#E102",
}

