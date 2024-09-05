package codes

// This is generated code. Do not modify. See gen folder.

import "github.com/srerickson/ocfl-go"

// E001: The OCFL Object Root must not contain files or directories other than those specified in the following sections.
func E001(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E001",
			Description: "The OCFL Object Root must not contain files or directories other than those specified in the following sections.",
			URL:         "https://ocfl.io/1.0/spec/#E001",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E001",
			Description: "The OCFL Object Root must not contain files or directories other than those specified in the following sections.",
			URL:         "https://ocfl.io/1.1/spec/#E001",
		}
	default:
		return nil
	}
}

// E002: The version declaration must be formatted according to the NAMASTE specification.
func E002(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E002",
			Description: "The version declaration must be formatted according to the NAMASTE specification.",
			URL:         "https://ocfl.io/1.0/spec/#E002",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E002",
			Description: "The version declaration must be formatted according to the NAMASTE specification.",
			URL:         "https://ocfl.io/1.1/spec/#E002",
		}
	default:
		return nil
	}
}

// E003: [The version declaration] must be a file in the base directory of the OCFL Object Root giving the OCFL version in the filename.
func E003(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E003",
			Description: "[The version declaration] must be a file in the base directory of the OCFL Object Root giving the OCFL version in the filename.",
			URL:         "https://ocfl.io/1.0/spec/#E003",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E003",
			Description: "There must be exactly one version declaration file in the base directory of the OCFL Object Root giving the OCFL version in the filename.",
			URL:         "https://ocfl.io/1.1/spec/#E003",
		}
	default:
		return nil
	}
}

// E004: The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification ocfl.Number.
func E004(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E004",
			Description: "The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E004",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E004",
			Description: "The [version declaration] filename MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E004",
		}
	default:
		return nil
	}
}

// E005: The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification ocfl.Number.
func E005(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E005",
			Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E005",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E005",
			Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_object_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E005",
		}
	default:
		return nil
	}
}

// E006: The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification ocfl.Number.
func E006(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E006",
			Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E006",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E006",
			Description: "The [version declaration] filename must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_object_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E006",
		}
	default:
		return nil
	}
}

// E007: The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).
func E007(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E007",
			Description: "The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).",
			URL:         "https://ocfl.io/1.0/spec/#E007",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E007",
			Description: "The text contents of the [version declaration] file must be the same as dvalue, followed by a newline (\n).",
			URL:         "https://ocfl.io/1.1/spec/#E007",
		}
	default:
		return nil
	}
}

// E008: OCFL Object content must be stored as a sequence of one or more versions.
func E008(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E008",
			Description: "OCFL Object content must be stored as a sequence of one or more versions.",
			URL:         "https://ocfl.io/1.0/spec/#E008",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E008",
			Description: "OCFL Object content must be stored as a sequence of one or more versions.",
			URL:         "https://ocfl.io/1.1/spec/#E008",
		}
	default:
		return nil
	}
}

// E009: The ocfl.Number sequence MUST start at 1 and must be continuous without missing integers.
func E009(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E009",
			Description: "The ocfl.Number sequence MUST start at 1 and must be continuous without missing integers.",
			URL:         "https://ocfl.io/1.0/spec/#E009",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E009",
			Description: "The version number sequence MUST start at 1 and must be continuous without missing integers.",
			URL:         "https://ocfl.io/1.1/spec/#E009",
		}
	default:
		return nil
	}
}

// E010: The ocfl.Number sequence must start at 1 and MUST be continuous without missing integers.
func E010(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E010",
			Description: "The ocfl.Number sequence must start at 1 and MUST be continuous without missing integers.",
			URL:         "https://ocfl.io/1.0/spec/#E010",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E010",
			Description: "The version number sequence must start at 1 and MUST be continuous without missing integers.",
			URL:         "https://ocfl.io/1.1/spec/#E010",
		}
	default:
		return nil
	}
}

// E011: If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.
func E011(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E011",
			Description: "If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.",
			URL:         "https://ocfl.io/1.0/spec/#E011",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E011",
			Description: "If zero-padded version directory numbers are used then they must start with the prefix v and then a zero.",
			URL:         "https://ocfl.io/1.1/spec/#E011",
		}
	default:
		return nil
	}
}

// E012: All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.
func E012(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E012",
			Description: "All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.",
			URL:         "https://ocfl.io/1.0/spec/#E012",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E012",
			Description: "All version directories of an object must use the same naming convention: either a non-padded version directory number, or a zero-padded version directory number of consistent length.",
			URL:         "https://ocfl.io/1.1/spec/#E012",
		}
	default:
		return nil
	}
}

// E013: Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.
func E013(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E013",
			Description: "Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.",
			URL:         "https://ocfl.io/1.0/spec/#E013",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E013",
			Description: "Operations that add a new version to an object must follow the version directory naming convention established by earlier versions.",
			URL:         "https://ocfl.io/1.1/spec/#E013",
		}
	default:
		return nil
	}
}

// E014: In all cases, references to files inside version directories from inventory files must use the actual version directory names.
func E014(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E014",
			Description: "In all cases, references to files inside version directories from inventory files must use the actual version directory names.",
			URL:         "https://ocfl.io/1.0/spec/#E014",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E014",
			Description: "In all cases, references to files inside version directories from inventory files must use the actual version directory names.",
			URL:         "https://ocfl.io/1.1/spec/#E014",
		}
	default:
		return nil
	}
}

// E015: There must be no other files as children of a version directory, other than an inventory file and a inventory digest.
func E015(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E015",
			Description: "There must be no other files as children of a version directory, other than an inventory file and a inventory digest.",
			URL:         "https://ocfl.io/1.0/spec/#E015",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E015",
			Description: "There must be no other files as children of a version directory, other than an inventory file and a inventory digest.",
			URL:         "https://ocfl.io/1.1/spec/#E015",
		}
	default:
		return nil
	}
}

// E016: Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.
func E016(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E016",
			Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.",
			URL:         "https://ocfl.io/1.0/spec/#E016",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E016",
			Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and should not contain this sub-directory otherwise.",
			URL:         "https://ocfl.io/1.1/spec/#E016",
		}
	default:
		return nil
	}
}

// E017: The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).
func E017(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E017",
			Description: "The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).",
			URL:         "https://ocfl.io/1.0/spec/#E017",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E017",
			Description: "The contentDirectory value MUST NOT contain the forward slash (/) path separator and must not be either one or two periods (. or ..).",
			URL:         "https://ocfl.io/1.1/spec/#E017",
		}
	default:
		return nil
	}
}

// E018: The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).
func E018(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E018",
			Description: "The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).",
			URL:         "https://ocfl.io/1.0/spec/#E018",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E018",
			Description: "The contentDirectory value must not contain the forward slash (/) path separator and MUST NOT be either one or two periods (. or ..).",
			URL:         "https://ocfl.io/1.1/spec/#E018",
		}
	default:
		return nil
	}
}

// E019: If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.
func E019(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E019",
			Description: "If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.",
			URL:         "https://ocfl.io/1.0/spec/#E019",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E019",
			Description: "If the key contentDirectory is set, it MUST be set in the first version of the object and must not change between versions of the same object.",
			URL:         "https://ocfl.io/1.1/spec/#E019",
		}
	default:
		return nil
	}
}

// E020: If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.
func E020(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E020",
			Description: "If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.",
			URL:         "https://ocfl.io/1.0/spec/#E020",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E020",
			Description: "If the key contentDirectory is set, it must be set in the first version of the object and MUST NOT change between versions of the same object.",
			URL:         "https://ocfl.io/1.1/spec/#E020",
		}
	default:
		return nil
	}
}

// E021: If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.
func E021(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E021",
			Description: "If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.",
			URL:         "https://ocfl.io/1.0/spec/#E021",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E021",
			Description: "If the key contentDirectory is not present in the inventory file then the name of the designated content sub-directory must be content.",
			URL:         "https://ocfl.io/1.1/spec/#E021",
		}
	default:
		return nil
	}
}

// E022: OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.
func E022(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E022",
			Description: "OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.",
			URL:         "https://ocfl.io/1.0/spec/#E022",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E022",
			Description: "OCFL-compliant tools (including any validators) must ignore all directories in the object version directory except for the designated content directory.",
			URL:         "https://ocfl.io/1.1/spec/#E022",
		}
	default:
		return nil
	}
}

// E023: Every file within a version's content directory must be referenced in the manifest section of the inventory.
func E023(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E023",
			Description: "Every file within a version's content directory must be referenced in the manifest section of the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E023",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E023",
			Description: "Every file within a version's content directory must be referenced in the manifest section of the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E023",
		}
	default:
		return nil
	}
}

// E024: There must not be empty directories within a version's content directory.
func E024(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E024",
			Description: "There must not be empty directories within a version's content directory.",
			URL:         "https://ocfl.io/1.0/spec/#E024",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E024",
			Description: "There must not be empty directories within a version's content directory.",
			URL:         "https://ocfl.io/1.1/spec/#E024",
		}
	default:
		return nil
	}
}

// E025: For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.
func E025(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E025",
			Description: "For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.",
			URL:         "https://ocfl.io/1.0/spec/#E025",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E025",
			Description: "For content-addressing, OCFL Objects must use either sha512 or sha256, and should use sha512.",
			URL:         "https://ocfl.io/1.1/spec/#E025",
		}
	default:
		return nil
	}
}

// E026: For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].
func E026(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E026",
			Description: "For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].",
			URL:         "https://ocfl.io/1.0/spec/#E026",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E026",
			Description: "For storage of additional fixity values, or to support legacy content migration, implementers must choose from the following controlled vocabulary of digest algorithms, or from a list of additional algorithms given in the [Digest-Algorithms-Extension].",
			URL:         "https://ocfl.io/1.1/spec/#E026",
		}
	default:
		return nil
	}
}

// E027: OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.
func E027(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E027",
			Description: "OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.",
			URL:         "https://ocfl.io/1.0/spec/#E027",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E027",
			Description: "OCFL clients must support all fixity algorithms given in the table below, and may support additional algorithms from the extensions.",
			URL:         "https://ocfl.io/1.1/spec/#E027",
		}
	default:
		return nil
	}
}

// E028: Optional fixity algorithms that are not supported by a client must be ignored by that client.
func E028(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E028",
			Description: "Optional fixity algorithms that are not supported by a client must be ignored by that client.",
			URL:         "https://ocfl.io/1.0/spec/#E028",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E028",
			Description: "Optional fixity algorithms that are not supported by a client must be ignored by that client.",
			URL:         "https://ocfl.io/1.1/spec/#E028",
		}
	default:
		return nil
	}
}

// E029: SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
func E029(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E029",
			Description: "SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.0/spec/#E029",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E029",
			Description: "SHA-1 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.1/spec/#E029",
		}
	default:
		return nil
	}
}

// E030: SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
func E030(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E030",
			Description: "SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.0/spec/#E030",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E030",
			Description: "SHA-256 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.1/spec/#E030",
		}
	default:
		return nil
	}
}

// E031: SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].
func E031(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E031",
			Description: "SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.0/spec/#E031",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E031",
			Description: "SHA-512 algorithm defined by [FIPS-180-4] and must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.1/spec/#E031",
		}
	default:
		return nil
	}
}

// E032: [blake2b-512] must be encoded using hex (base16) encoding [RFC4648].
func E032(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E032",
			Description: "[blake2b-512] must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.0/spec/#E032",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E032",
			Description: "[blake2b-512] must be encoded using hex (base16) encoding [RFC4648].",
			URL:         "https://ocfl.io/1.1/spec/#E032",
		}
	default:
		return nil
	}
}

// E033: An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.
func E033(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E033",
			Description: "An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.",
			URL:         "https://ocfl.io/1.0/spec/#E033",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E033",
			Description: "An OCFL Object Inventory MUST follow the [JSON] structure described in this section and must be named inventory.json.",
			URL:         "https://ocfl.io/1.1/spec/#E033",
		}
	default:
		return nil
	}
}

// E034: An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.
func E034(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E034",
			Description: "An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.",
			URL:         "https://ocfl.io/1.0/spec/#E034",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E034",
			Description: "An OCFL Object Inventory must follow the [JSON] structure described in this section and MUST be named inventory.json.",
			URL:         "https://ocfl.io/1.1/spec/#E034",
		}
	default:
		return nil
	}
}

// E035: The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.
func E035(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E035",
			Description: "The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E035",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E035",
			Description: "The forward slash (/) path separator must be used in content paths in the manifest and fixity blocks within the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E035",
		}
	default:
		return nil
	}
}

// E036: An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]
func E036(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E036",
			Description: "An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]",
			URL:         "https://ocfl.io/1.0/spec/#E036",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E036",
			Description: "An OCFL Object Inventory must include the following keys: [id, type, digestAlgorithm, head]",
			URL:         "https://ocfl.io/1.1/spec/#E036",
		}
	default:
		return nil
	}
}

// E037: [id] must be unique in the local context, and should be a URI [RFC3986].
func E037(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E037",
			Description: "[id] must be unique in the local context, and should be a URI [RFC3986].",
			URL:         "https://ocfl.io/1.0/spec/#E037",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E037",
			Description: "[id] must be unique in the local context, and should be a URI [RFC3986].",
			URL:         "https://ocfl.io/1.1/spec/#E037",
		}
	default:
		return nil
	}
}

// E038: In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.
func E038(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E038",
			Description: "In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.",
			URL:         "https://ocfl.io/1.0/spec/#E038",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E038",
			Description: "In the object root inventory [the type value] must be the URI of the inventory section of the specification version matching the object conformance declaration.",
			URL:         "https://ocfl.io/1.1/spec/#E038",
		}
	default:
		return nil
	}
}

// E039: [digestAlgorithm] must be the algorithm used in the manifest and state blocks.
func E039(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E039",
			Description: "[digestAlgorithm] must be the algorithm used in the manifest and state blocks.",
			URL:         "https://ocfl.io/1.0/spec/#E039",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E039",
			Description: "[digestAlgorithm] must be the algorithm used in the manifest and state blocks.",
			URL:         "https://ocfl.io/1.1/spec/#E039",
		}
	default:
		return nil
	}
}

// E040: [head] must be the version directory name with the highest ocfl.Number.
func E040(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E040",
			Description: "[head] must be the version directory name with the highest ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E040",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E040",
			Description: "[head] must be the version directory name with the highest version number.",
			URL:         "https://ocfl.io/1.1/spec/#E040",
		}
	default:
		return nil
	}
}

// E041: In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.
func E041(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E041",
			Description: "In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.",
			URL:         "https://ocfl.io/1.0/spec/#E041",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E041",
			Description: "In addition to these keys, there must be two other blocks present, manifest and versions, which are discussed in the next two sections.",
			URL:         "https://ocfl.io/1.1/spec/#E041",
		}
	default:
		return nil
	}
}

// E042: Content paths within a manifest block must be relative to the OCFL Object Root.
func E042(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E042",
			Description: "Content paths within a manifest block must be relative to the OCFL Object Root.",
			URL:         "https://ocfl.io/1.0/spec/#E042",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E042",
			Description: "Content paths within a manifest block must be relative to the OCFL Object Root.",
			URL:         "https://ocfl.io/1.1/spec/#E042",
		}
	default:
		return nil
	}
}

// E043: An OCFL Object Inventory must include a block for storing versions.
func E043(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E043",
			Description: "An OCFL Object Inventory must include a block for storing versions.",
			URL:         "https://ocfl.io/1.0/spec/#E043",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E043",
			Description: "An OCFL Object Inventory must include a block for storing versions.",
			URL:         "https://ocfl.io/1.1/spec/#E043",
		}
	default:
		return nil
	}
}

// E044: This block MUST have the key of versions within the inventory, and it must be a JSON object.
func E044(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E044",
			Description: "This block MUST have the key of versions within the inventory, and it must be a JSON object.",
			URL:         "https://ocfl.io/1.0/spec/#E044",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E044",
			Description: "This block MUST have the key of versions within the inventory, and it must be a JSON object.",
			URL:         "https://ocfl.io/1.1/spec/#E044",
		}
	default:
		return nil
	}
}

// E045: This block must have the key of versions within the inventory, and it MUST be a JSON object.
func E045(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E045",
			Description: "This block must have the key of versions within the inventory, and it MUST be a JSON object.",
			URL:         "https://ocfl.io/1.0/spec/#E045",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E045",
			Description: "This block must have the key of versions within the inventory, and it MUST be a JSON object.",
			URL:         "https://ocfl.io/1.1/spec/#E045",
		}
	default:
		return nil
	}
}

// E046: The keys of [the versions object] must correspond to the names of the version directories used.
func E046(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E046",
			Description: "The keys of [the versions object] must correspond to the names of the version directories used.",
			URL:         "https://ocfl.io/1.0/spec/#E046",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E046",
			Description: "The keys of [the versions object] must correspond to the names of the version directories used.",
			URL:         "https://ocfl.io/1.1/spec/#E046",
		}
	default:
		return nil
	}
}

// E047: Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.
func E047(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E047",
			Description: "Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.",
			URL:         "https://ocfl.io/1.0/spec/#E047",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E047",
			Description: "Each value [of the versions object] must be another JSON object that characterizes the version, as described in the 3.5.3.1 Version section.",
			URL:         "https://ocfl.io/1.1/spec/#E047",
		}
	default:
		return nil
	}
}

// E048: A JSON object to describe one OCFL Version, which must include the following keys: [created, state]
func E048(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E048",
			Description: "A JSON object to describe one OCFL Version, which must include the following keys: [created, state]",
			URL:         "https://ocfl.io/1.0/spec/#E048",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E048",
			Description: "A JSON object to describe one OCFL Version, which must include the following keys: [created, state]",
			URL:         "https://ocfl.io/1.1/spec/#E048",
		}
	default:
		return nil
	}
}

// E049: [the value of the \"created\" key] must be expressed in the Internet Date/Time Format defined by [RFC3339].
func E049(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E049",
			Description: "[the value of the \"created\" key] must be expressed in the Internet Date/Time Format defined by [RFC3339].",
			URL:         "https://ocfl.io/1.0/spec/#E049",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E049",
			Description: "[the value of the “created” key] must be expressed in the Internet Date/Time Format defined by [RFC3339].",
			URL:         "https://ocfl.io/1.1/spec/#E049",
		}
	default:
		return nil
	}
}

// E050: The keys of [the \"state\" JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.
func E050(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E050",
			Description: "The keys of [the \"state\" JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E050",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E050",
			Description: "The keys of [the “state” JSON object] are digest values, each of which must correspond to an entry in the manifest of the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E050",
		}
	default:
		return nil
	}
}

// E051: The logical path [value of a \"state\" digest key] must be interpreted as a set of one or more path elements joined by a / path separator.
func E051(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E051",
			Description: "The logical path [value of a \"state\" digest key] must be interpreted as a set of one or more path elements joined by a / path separator.",
			URL:         "https://ocfl.io/1.0/spec/#E051",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E051",
			Description: "The logical path [value of a “state” digest key] must be interpreted as a set of one or more path elements joined by a / path separator.",
			URL:         "https://ocfl.io/1.1/spec/#E051",
		}
	default:
		return nil
	}
}

// E052: [logical] Path elements must not be ., .., or empty (//).
func E052(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E052",
			Description: "[logical] Path elements must not be ., .., or empty (//).",
			URL:         "https://ocfl.io/1.0/spec/#E052",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E052",
			Description: "[logical] Path elements must not be ., .., or empty (//).",
			URL:         "https://ocfl.io/1.1/spec/#E052",
		}
	default:
		return nil
	}
}

// E053: Additionally, a logical path must not begin or end with a forward slash (/).
func E053(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E053",
			Description: "Additionally, a logical path must not begin or end with a forward slash (/).",
			URL:         "https://ocfl.io/1.0/spec/#E053",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E053",
			Description: "Additionally, a logical path must not begin or end with a forward slash (/).",
			URL:         "https://ocfl.io/1.1/spec/#E053",
		}
	default:
		return nil
	}
}

// E054: The value of the user key must contain a user name key, \"name\" and should contain an address key, \"address\".
func E054(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E054",
			Description: "The value of the user key must contain a user name key, \"name\" and should contain an address key, \"address\".",
			URL:         "https://ocfl.io/1.0/spec/#E054",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E054",
			Description: "The value of the user key must contain a user name key, “name” and should contain an address key, “address”.",
			URL:         "https://ocfl.io/1.1/spec/#E054",
		}
	default:
		return nil
	}
}

// E055: This block must have the key of fixity within the inventory.
func E055(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E055",
			Description: "This block must have the key of fixity within the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E055",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E055",
			Description: "If present, [the fixity] block must have the key of fixity within the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E055",
		}
	default:
		return nil
	}
}

// E056: The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.
func E056(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E056",
			Description: "The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.",
			URL:         "https://ocfl.io/1.0/spec/#E056",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E056",
			Description: "The fixity block must contain keys corresponding to the controlled vocabulary given in the digest algorithms listed in the Digests section, or in a table given in an Extension.",
			URL:         "https://ocfl.io/1.1/spec/#E056",
		}
	default:
		return nil
	}
}

// E057: The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.
func E057(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E057",
			Description: "The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.",
			URL:         "https://ocfl.io/1.0/spec/#E057",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E057",
			Description: "The value of the fixity block for a particular digest algorithm must follow the structure of the manifest block; that is, a key corresponding to the digest value, and an array of content paths that match that digest.",
			URL:         "https://ocfl.io/1.1/spec/#E057",
		}
	default:
		return nil
	}
}

// E058: Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.
func E058(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E058",
			Description: "Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.",
			URL:         "https://ocfl.io/1.0/spec/#E058",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E058",
			Description: "Every occurrence of an inventory file must have an accompanying sidecar file stating its digest.",
			URL:         "https://ocfl.io/1.1/spec/#E058",
		}
	default:
		return nil
	}
}

// E059: This value must match the value given for the digestAlgorithm key in the inventory.
func E059(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E059",
			Description: "This value must match the value given for the digestAlgorithm key in the inventory.",
			URL:         "https://ocfl.io/1.0/spec/#E059",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E059",
			Description: "This value must match the value given for the digestAlgorithm key in the inventory.",
			URL:         "https://ocfl.io/1.1/spec/#E059",
		}
	default:
		return nil
	}
}

// E060: The digest sidecar file must contain the digest of the inventory file.
func E060(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E060",
			Description: "The digest sidecar file must contain the digest of the inventory file.",
			URL:         "https://ocfl.io/1.0/spec/#E060",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E060",
			Description: "The digest sidecar file must contain the digest of the inventory file.",
			URL:         "https://ocfl.io/1.1/spec/#E060",
		}
	default:
		return nil
	}
}

// E061: [The digest sidecar file] must follow the format: DIGEST inventory.json
func E061(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E061",
			Description: "[The digest sidecar file] must follow the format: DIGEST inventory.json",
			URL:         "https://ocfl.io/1.0/spec/#E061",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E061",
			Description: "[The digest sidecar file] must follow the format: DIGEST inventory.json",
			URL:         "https://ocfl.io/1.1/spec/#E061",
		}
	default:
		return nil
	}
}

// E062: The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.
func E062(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E062",
			Description: "The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.",
			URL:         "https://ocfl.io/1.0/spec/#E062",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E062",
			Description: "The digest of the inventory must be computed only after all changes to the inventory have been made, and thus writing the digest sidecar file is the last step in the versioning process.",
			URL:         "https://ocfl.io/1.1/spec/#E062",
		}
	default:
		return nil
	}
}

// E063: Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.
func E063(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E063",
			Description: "Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.",
			URL:         "https://ocfl.io/1.0/spec/#E063",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E063",
			Description: "Every OCFL Object must have an inventory file within the OCFL Object Root, corresponding to the state of the OCFL Object at the current version.",
			URL:         "https://ocfl.io/1.1/spec/#E063",
		}
	default:
		return nil
	}
}

// E064: Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.
func E064(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E064",
			Description: "Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.",
			URL:         "https://ocfl.io/1.0/spec/#E064",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E064",
			Description: "Where an OCFL Object contains inventory.json in version directories, the inventory file in the OCFL Object Root must be the same as the file in the most recent version.",
			URL:         "https://ocfl.io/1.1/spec/#E064",
		}
	default:
		return nil
	}
}

// E066: Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.
func E066(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E066",
			Description: "Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.",
			URL:         "https://ocfl.io/1.0/spec/#E066",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E066",
			Description: "Each version block in each prior inventory file must represent the same object state as the corresponding version block in the current inventory file.",
			URL:         "https://ocfl.io/1.1/spec/#E066",
		}
	default:
		return nil
	}
}

// E067: The extensions directory must not contain any files, and no sub-directories other than extension sub-directories.
func E067(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E067",
			Description: "The extensions directory must not contain any files, and no sub-directories other than extension sub-directories.",
			URL:         "https://ocfl.io/1.0/spec/#E067",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E067",
			Description: "The extensions directory must not contain any files or sub-directories other than extension sub-directories.",
			URL:         "https://ocfl.io/1.1/spec/#E067",
		}
	default:
		return nil
	}
}

// E068: The specific structure and function of the extension, as well as a declaration of the registered extension name must be defined in one of the following locations: The OCFL Extensions repository OR The Storage Root, as a plain text document directly in the Storage Root.
func E068(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E068",
			Description: "The specific structure and function of the extension, as well as a declaration of the registered extension name must be defined in one of the following locations: The OCFL Extensions repository OR The Storage Root, as a plain text document directly in the Storage Root.",
			URL:         "https://ocfl.io/1.0/spec/#E068",
		}
	default:
		return nil
	}
}

// E069: An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.
func E069(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E069",
			Description: "An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.",
			URL:         "https://ocfl.io/1.0/spec/#E069",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E069",
			Description: "An OCFL Storage Root MUST contain a Root Conformance Declaration identifying it as such.",
			URL:         "https://ocfl.io/1.1/spec/#E069",
		}
	default:
		return nil
	}
}

// E070: If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [key, description]
func E070(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E070",
			Description: "If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [key, description]",
			URL:         "https://ocfl.io/1.0/spec/#E070",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E070",
			Description: "If present, [the ocfl_layout.json document] MUST include the following two keys in the root JSON object: [extension, description]",
			URL:         "https://ocfl.io/1.1/spec/#E070",
		}
	default:
		return nil
	}
}

// E071: The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.
func E071(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E071",
			Description: "The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.",
			URL:         "https://ocfl.io/1.0/spec/#E071",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E071",
			Description: "The value of the [ocfl_layout.json] extension key must be the registered extension name for the extension defining the arrangement under the storage root.",
			URL:         "https://ocfl.io/1.1/spec/#E071",
		}
	default:
		return nil
	}
}

// E072: The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.
func E072(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E072",
			Description: "The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.",
			URL:         "https://ocfl.io/1.0/spec/#E072",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E072",
			Description: "The directory hierarchy used to store OCFL Objects MUST NOT contain files that are not part of an OCFL Object.",
			URL:         "https://ocfl.io/1.1/spec/#E072",
		}
	default:
		return nil
	}
}

// E073: Empty directories MUST NOT appear under a storage root.
func E073(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E073",
			Description: "Empty directories MUST NOT appear under a storage root.",
			URL:         "https://ocfl.io/1.0/spec/#E073",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E073",
			Description: "Empty directories MUST NOT appear under a storage root.",
			URL:         "https://ocfl.io/1.1/spec/#E073",
		}
	default:
		return nil
	}
}

// E074: Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple \"buckets\" in an object store - each OCFL Storage Root MUST be independent.
func E074(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E074",
			Description: "Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple \"buckets\" in an object store - each OCFL Storage Root MUST be independent.",
			URL:         "https://ocfl.io/1.0/spec/#E074",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E074",
			Description: "Although implementations may require multiple OCFL Storage Roots - that is, several logical or physical volumes, or multiple “buckets” in an object store - each OCFL Storage Root MUST be independent.",
			URL:         "https://ocfl.io/1.1/spec/#E074",
		}
	default:
		return nil
	}
}

// E075: The OCFL version declaration MUST be formatted according to the NAMASTE specification.
func E075(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E075",
			Description: "The OCFL version declaration MUST be formatted according to the NAMASTE specification.",
			URL:         "https://ocfl.io/1.0/spec/#E075",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E075",
			Description: "The OCFL version declaration MUST be formatted according to the NAMASTE specification.",
			URL:         "https://ocfl.io/1.1/spec/#E075",
		}
	default:
		return nil
	}
}

// E076: [The OCFL version declaration] MUST be a file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.
func E076(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E076",
			Description: "[The OCFL version declaration] MUST be a file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.",
			URL:         "https://ocfl.io/1.0/spec/#E076",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E076",
			Description: "There must be exactly one version declaration file in the base directory of the OCFL Storage Root giving the OCFL version in the filename.",
			URL:         "https://ocfl.io/1.1/spec/#E076",
		}
	default:
		return nil
	}
}

// E077: [The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification ocfl.Number.
func E077(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E077",
			Description: "[The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E077",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E077",
			Description: "[The OCFL version declaration filename] MUST conform to the pattern T=dvalue, where T must be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E077",
		}
	default:
		return nil
	}
}

// E078: [The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification ocfl.Number.
func E078(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E078",
			Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E078",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E078",
			Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T MUST be 0, and dvalue must be ocfl_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E078",
		}
	default:
		return nil
	}
}

// E079: [The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification ocfl.Number.
func E079(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E079",
			Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification ocfl.Number.",
			URL:         "https://ocfl.io/1.0/spec/#E079",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E079",
			Description: "[The OCFL version declaration filename] must conform to the pattern T=dvalue, where T must be 0, and dvalue MUST be ocfl_, followed by the OCFL specification version number.",
			URL:         "https://ocfl.io/1.1/spec/#E079",
		}
	default:
		return nil
	}
}

// E080: The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).
func E080(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E080",
			Description: "The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).",
			URL:         "https://ocfl.io/1.0/spec/#E080",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E080",
			Description: "The text contents of [the OCFL version declaration file] MUST be the same as dvalue, followed by a newline (\n).",
			URL:         "https://ocfl.io/1.1/spec/#E080",
		}
	default:
		return nil
	}
}

// E081: OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.
func E081(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E081",
			Description: "OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.",
			URL:         "https://ocfl.io/1.0/spec/#E081",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E081",
			Description: "OCFL Objects within the OCFL Storage Root also include a conformance declaration which MUST indicate OCFL Object conformance to the same or earlier version of the specification.",
			URL:         "https://ocfl.io/1.1/spec/#E081",
		}
	default:
		return nil
	}
}

// E082: OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.
func E082(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E082",
			Description: "OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.",
			URL:         "https://ocfl.io/1.0/spec/#E082",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E082",
			Description: "OCFL Object Roots MUST be stored either as the terminal resource at the end of a directory storage hierarchy or as direct children of a containing OCFL Storage Root.",
			URL:         "https://ocfl.io/1.1/spec/#E082",
		}
	default:
		return nil
	}
}

// E083: There MUST be a deterministic mapping from an object identifier to a unique storage path.
func E083(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E083",
			Description: "There MUST be a deterministic mapping from an object identifier to a unique storage path.",
			URL:         "https://ocfl.io/1.0/spec/#E083",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E083",
			Description: "There MUST be a deterministic mapping from an object identifier to a unique storage path.",
			URL:         "https://ocfl.io/1.1/spec/#E083",
		}
	default:
		return nil
	}
}

// E084: Storage hierarchies MUST NOT include files within intermediate directories.
func E084(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E084",
			Description: "Storage hierarchies MUST NOT include files within intermediate directories.",
			URL:         "https://ocfl.io/1.0/spec/#E084",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E084",
			Description: "Storage hierarchies MUST NOT include files within intermediate directories.",
			URL:         "https://ocfl.io/1.1/spec/#E084",
		}
	default:
		return nil
	}
}

// E085: Storage hierarchies MUST be terminated by OCFL Object Roots.
func E085(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E085",
			Description: "Storage hierarchies MUST be terminated by OCFL Object Roots.",
			URL:         "https://ocfl.io/1.0/spec/#E085",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E085",
			Description: "Storage hierarchies MUST be terminated by OCFL Object Roots.",
			URL:         "https://ocfl.io/1.1/spec/#E085",
		}
	default:
		return nil
	}
}

// E086: The storage root extensions directory MUST conform to the same guidelines and limitations as those defined for object extensions.
func E086(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E086",
			Description: "The storage root extensions directory MUST conform to the same guidelines and limitations as those defined for object extensions.",
			URL:         "https://ocfl.io/1.0/spec/#E086",
		}
	default:
		return nil
	}
}

// E087: An OCFL validator MUST ignore any files in the storage root it does not understand.
func E087(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E087",
			Description: "An OCFL validator MUST ignore any files in the storage root it does not understand.",
			URL:         "https://ocfl.io/1.0/spec/#E087",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E087",
			Description: "An OCFL validator MUST ignore any files in the storage root it does not understand.",
			URL:         "https://ocfl.io/1.1/spec/#E087",
		}
	default:
		return nil
	}
}

// E088: An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.
func E088(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E088",
			Description: "An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.",
			URL:         "https://ocfl.io/1.0/spec/#E088",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E088",
			Description: "An OCFL Storage Root MUST NOT contain directories or sub-directories other than as a directory hierarchy used to store OCFL Objects or for storage root extensions.",
			URL:         "https://ocfl.io/1.1/spec/#E088",
		}
	default:
		return nil
	}
}

// E089: If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.
func E089(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E089",
			Description: "If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.",
			URL:         "https://ocfl.io/1.0/spec/#E089",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E089",
			Description: "If the preservation of non-OCFL-compliant features is required then the content MUST be wrapped in a suitable disk or filesystem image format which OCFL can treat as a regular file.",
			URL:         "https://ocfl.io/1.1/spec/#E089",
		}
	default:
		return nil
	}
}

// E090: Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierachies.
func E090(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E090",
			Description: "Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierachies.",
			URL:         "https://ocfl.io/1.0/spec/#E090",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E090",
			Description: "Hard and soft (symbolic) links are not portable and MUST NOT be used within OCFL Storage hierarchies.",
			URL:         "https://ocfl.io/1.1/spec/#E090",
		}
	default:
		return nil
	}
}

// E091: Filesystems MUST preserve the case of OCFL filepaths and filenames.
func E091(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E091",
			Description: "Filesystems MUST preserve the case of OCFL filepaths and filenames.",
			URL:         "https://ocfl.io/1.0/spec/#E091",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E091",
			Description: "Filesystems MUST preserve the case of OCFL filepaths and filenames.",
			URL:         "https://ocfl.io/1.1/spec/#E091",
		}
	default:
		return nil
	}
}

// E092: The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.
func E092(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E092",
			Description: "The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.",
			URL:         "https://ocfl.io/1.0/spec/#E092",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E092",
			Description: "The value for each key in the manifest must be an array containing the content paths of files in the OCFL Object that have content with the given digest.",
			URL:         "https://ocfl.io/1.1/spec/#E092",
		}
	default:
		return nil
	}
}

// E093: Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.
func E093(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E093",
			Description: "Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.",
			URL:         "https://ocfl.io/1.0/spec/#E093",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E093",
			Description: "Where included in the fixity block, the digest values given must match the digests of the files at the corresponding content paths.",
			URL:         "https://ocfl.io/1.1/spec/#E093",
		}
	default:
		return nil
	}
}

// E094: The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.
func E094(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E094",
			Description: "The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.",
			URL:         "https://ocfl.io/1.0/spec/#E094",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E094",
			Description: "The value of [the message] key is freeform text, used to record the rationale for creating this version. It must be a JSON string.",
			URL:         "https://ocfl.io/1.1/spec/#E094",
		}
	default:
		return nil
	}
}

// E095: Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.
func E095(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E095",
			Description: "Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.",
			URL:         "https://ocfl.io/1.0/spec/#E095",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E095",
			Description: "Within a version, logical paths must be unique and non-conflicting, so the logical path for a file cannot appear as the initial part of another logical path.",
			URL:         "https://ocfl.io/1.1/spec/#E095",
		}
	default:
		return nil
	}
}

// E096: As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.
func E096(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E096",
			Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.",
			URL:         "https://ocfl.io/1.0/spec/#E096",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E096",
			Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the manifest regardless of case.",
			URL:         "https://ocfl.io/1.1/spec/#E096",
		}
	default:
		return nil
	}
}

// E097: As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.
func E097(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E097",
			Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.",
			URL:         "https://ocfl.io/1.0/spec/#E097",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E097",
			Description: "As JSON keys are case sensitive, while digests may not be, there is an additional requirement that each digest value must occur only once in the fixity block for any digest algorithm, regardless of case.",
			URL:         "https://ocfl.io/1.1/spec/#E097",
		}
	default:
		return nil
	}
}

// E098: The content path must be interpreted as a set of one or more path elements joined by a / path separator.
func E098(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E098",
			Description: "The content path must be interpreted as a set of one or more path elements joined by a / path separator.",
			URL:         "https://ocfl.io/1.0/spec/#E098",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E098",
			Description: "The content path must be interpreted as a set of one or more path elements joined by a / path separator.",
			URL:         "https://ocfl.io/1.1/spec/#E098",
		}
	default:
		return nil
	}
}

// E099: [content] path elements must not be ., .., or empty (//).
func E099(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E099",
			Description: "[content] path elements must not be ., .., or empty (//).",
			URL:         "https://ocfl.io/1.0/spec/#E099",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E099",
			Description: "[content] path elements must not be ., .., or empty (//).",
			URL:         "https://ocfl.io/1.1/spec/#E099",
		}
	default:
		return nil
	}
}

// E100: A content path must not begin or end with a forward slash (/).
func E100(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E100",
			Description: "A content path must not begin or end with a forward slash (/).",
			URL:         "https://ocfl.io/1.0/spec/#E100",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E100",
			Description: "A content path must not begin or end with a forward slash (/).",
			URL:         "https://ocfl.io/1.1/spec/#E100",
		}
	default:
		return nil
	}
}

// E101: Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.
func E101(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E101",
			Description: "Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.",
			URL:         "https://ocfl.io/1.0/spec/#E101",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E101",
			Description: "Within an inventory, content paths must be unique and non-conflicting, so the content path for a file cannot appear as the initial part of another content path.",
			URL:         "https://ocfl.io/1.1/spec/#E101",
		}
	default:
		return nil
	}
}

// E102: An inventory file must not contain keys that are not specified.
func E102(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E102",
			Description: "An inventory file must not contain keys that are not specified.",
			URL:         "https://ocfl.io/1.0/spec/#E102",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E102",
			Description: "An inventory file must not contain keys that are not specified.",
			URL:         "https://ocfl.io/1.1/spec/#E102",
		}
	default:
		return nil
	}
}

// E103: Each version directory within an OCFL Object MUST conform to either the same or a later OCFL specification version as the preceding version directory.
func E103(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E103",
			Description: "Each version directory within an OCFL Object MUST conform to either the same or a later OCFL specification version as the preceding version directory.",
			URL:         "https://ocfl.io/1.1/spec/#E103",
		}
	default:
		return nil
	}
}

// E104: Version directory names MUST be constructed by prepending v to the version number.
func E104(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E104",
			Description: "Version directory names MUST be constructed by prepending v to the version number.",
			URL:         "https://ocfl.io/1.1/spec/#E104",
		}
	default:
		return nil
	}
}

// E105: The version number MUST be taken from the sequence of positive, base-ten integers: 1, 2, 3, etc.
func E105(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E105",
			Description: "The version number MUST be taken from the sequence of positive, base-ten integers: 1, 2, 3, etc.",
			URL:         "https://ocfl.io/1.1/spec/#E105",
		}
	default:
		return nil
	}
}

// E106: The value of the manifest key MUST be a JSON object.
func E106(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E106",
			Description: "The value of the manifest key MUST be a JSON object.",
			URL:         "https://ocfl.io/1.1/spec/#E106",
		}
	default:
		return nil
	}
}

// E107: The value of the manifest key must be a JSON object, and each key MUST correspond to a digest value key found in one or more state blocks of the current and/or previous version blocks of the OCFL Object.
func E107(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E107",
			Description: "The value of the manifest key must be a JSON object, and each key MUST correspond to a digest value key found in one or more state blocks of the current and/or previous version blocks of the OCFL Object.",
			URL:         "https://ocfl.io/1.1/spec/#E107",
		}
	default:
		return nil
	}
}

// E108: The contentDirectory value MUST represent a direct child directory of the version directory in which it is found.
func E108(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E108",
			Description: "The contentDirectory value MUST represent a direct child directory of the version directory in which it is found.",
			URL:         "https://ocfl.io/1.1/spec/#E108",
		}
	default:
		return nil
	}
}

// E110: A unique identifier for the OCFL Object MUST NOT change between versions of the same object.
func E110(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E110",
			Description: "A unique identifier for the OCFL Object MUST NOT change between versions of the same object.",
			URL:         "https://ocfl.io/1.1/spec/#E110",
		}
	default:
		return nil
	}
}

// E111: If present, [the value of the fixity key] MUST be a JSON object, which may be empty.
func E111(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E111",
			Description: "If present, [the value of the fixity key] MUST be a JSON object, which may be empty.",
			URL:         "https://ocfl.io/1.1/spec/#E111",
		}
	default:
		return nil
	}
}

// E112: The extensions directory must not contain any files or sub-directories other than extension sub-directories.
func E112(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "E112",
			Description: "The extensions directory must not contain any files or sub-directories other than extension sub-directories.",
			URL:         "https://ocfl.io/1.1/spec/#E112",
		}
	default:
		return nil
	}
}

// W001: Implementations SHOULD use version directory names constructed without zero-padding the ocfl.Number, ie. v1, v2, v3, etc.
func W001(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W001",
			Description: "Implementations SHOULD use version directory names constructed without zero-padding the ocfl.Number, ie. v1, v2, v3, etc.",
			URL:         "https://ocfl.io/1.0/spec/#W001",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W001",
			Description: "Implementations SHOULD use version directory names constructed without zero-padding the version number, ie. v1, v2, v3, etc.",
			URL:         "https://ocfl.io/1.1/spec/#W001",
		}
	default:
		return nil
	}
}

// W002: The version directory SHOULD NOT contain any directories other than the designated content sub-directory. Once created, the contents of a version directory are expected to be immutable.
func W002(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W002",
			Description: "The version directory SHOULD NOT contain any directories other than the designated content sub-directory. Once created, the contents of a version directory are expected to be immutable.",
			URL:         "https://ocfl.io/1.0/spec/#W002",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W002",
			Description: "The version directory SHOULD NOT contain any directories other than the designated content sub-directory. Once created, the contents of a version directory are expected to be immutable.",
			URL:         "https://ocfl.io/1.1/spec/#W002",
		}
	default:
		return nil
	}
}

// W003: Version directories must contain a designated content sub-directory if the version contains files to be preserved, and SHOULD NOT contain this sub-directory otherwise.
func W003(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W003",
			Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and SHOULD NOT contain this sub-directory otherwise.",
			URL:         "https://ocfl.io/1.0/spec/#W003",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W003",
			Description: "Version directories must contain a designated content sub-directory if the version contains files to be preserved, and SHOULD NOT contain this sub-directory otherwise.",
			URL:         "https://ocfl.io/1.1/spec/#W003",
		}
	default:
		return nil
	}
}

// W004: For content-addressing, OCFL Objects SHOULD use sha512.
func W004(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W004",
			Description: "For content-addressing, OCFL Objects SHOULD use sha512.",
			URL:         "https://ocfl.io/1.0/spec/#W004",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W004",
			Description: "For content-addressing, OCFL Objects SHOULD use sha512.",
			URL:         "https://ocfl.io/1.1/spec/#W004",
		}
	default:
		return nil
	}
}

// W005: The OCFL Object Inventory id SHOULD be a URI.
func W005(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W005",
			Description: "The OCFL Object Inventory id SHOULD be a URI.",
			URL:         "https://ocfl.io/1.0/spec/#W005",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W005",
			Description: "The OCFL Object Inventory id SHOULD be a URI.",
			URL:         "https://ocfl.io/1.1/spec/#W005",
		}
	default:
		return nil
	}
}

// W007: In the OCFL Object Inventory, the JSON object describing an OCFL Version, SHOULD include the message and user keys.
func W007(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W007",
			Description: "In the OCFL Object Inventory, the JSON object describing an OCFL Version, SHOULD include the message and user keys.",
			URL:         "https://ocfl.io/1.0/spec/#W007",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W007",
			Description: "In the OCFL Object Inventory, the JSON object describing an OCFL Version, SHOULD include the message and user keys.",
			URL:         "https://ocfl.io/1.1/spec/#W007",
		}
	default:
		return nil
	}
}

// W008: In the OCFL Object Inventory, in the version block, the value of the user key SHOULD contain an address key, address.
func W008(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W008",
			Description: "In the OCFL Object Inventory, in the version block, the value of the user key SHOULD contain an address key, address.",
			URL:         "https://ocfl.io/1.0/spec/#W008",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W008",
			Description: "In the OCFL Object Inventory, in the version block, the value of the user key SHOULD contain an address key, address.",
			URL:         "https://ocfl.io/1.1/spec/#W008",
		}
	default:
		return nil
	}
}

// W009: In the OCFL Object Inventory, in the version block, the address value SHOULD be a URI: either a mailto URI [RFC6068] with the e-mail address of the user or a URL to a personal identifier, e.g., an ORCID iD.
func W009(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W009",
			Description: "In the OCFL Object Inventory, in the version block, the address value SHOULD be a URI: either a mailto URI [RFC6068] with the e-mail address of the user or a URL to a personal identifier, e.g., an ORCID iD.",
			URL:         "https://ocfl.io/1.0/spec/#W009",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W009",
			Description: "In the OCFL Object Inventory, in the version block, the address value SHOULD be a URI: either a mailto URI [RFC6068] with the e-mail address of the user or a URL to a personal identifier, e.g., an ORCID iD.",
			URL:         "https://ocfl.io/1.1/spec/#W009",
		}
	default:
		return nil
	}
}

// W010: In addition to the inventory in the OCFL Object Root, every version directory SHOULD include an inventory file that is an Inventory of all content for versions up to and including that particular version.
func W010(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W010",
			Description: "In addition to the inventory in the OCFL Object Root, every version directory SHOULD include an inventory file that is an Inventory of all content for versions up to and including that particular version.",
			URL:         "https://ocfl.io/1.0/spec/#W010",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W010",
			Description: "In addition to the inventory in the OCFL Object Root, every version directory SHOULD include an inventory file that is an Inventory of all content for versions up to and including that particular version.",
			URL:         "https://ocfl.io/1.1/spec/#W010",
		}
	default:
		return nil
	}
}

// W011: In the case that prior version directories include an inventory file, the values of the created, message and user keys in each version block in each prior inventory file SHOULD have the same values as the corresponding keys in the corresponding version block in the current inventory file.
func W011(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W011",
			Description: "In the case that prior version directories include an inventory file, the values of the created, message and user keys in each version block in each prior inventory file SHOULD have the same values as the corresponding keys in the corresponding version block in the current inventory file.",
			URL:         "https://ocfl.io/1.0/spec/#W011",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W011",
			Description: "In the case that prior version directories include an inventory file, the values of the created, message and user keys in each version block in each prior inventory file SHOULD have the same values as the corresponding keys in the corresponding version block in the current inventory file.",
			URL:         "https://ocfl.io/1.1/spec/#W011",
		}
	default:
		return nil
	}
}

// W012: Implementers SHOULD use the logs directory, if present, for storing files that contain a record of actions taken on the object.
func W012(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W012",
			Description: "Implementers SHOULD use the logs directory, if present, for storing files that contain a record of actions taken on the object.",
			URL:         "https://ocfl.io/1.0/spec/#W012",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W012",
			Description: "Implementers SHOULD use the logs directory, if present, for storing files that contain a record of actions taken on the object.",
			URL:         "https://ocfl.io/1.1/spec/#W012",
		}
	default:
		return nil
	}
}

// W013: In an OCFL Object, extension sub-directories SHOULD be named according to a registered extension name.
func W013(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W013",
			Description: "In an OCFL Object, extension sub-directories SHOULD be named according to a registered extension name.",
			URL:         "https://ocfl.io/1.0/spec/#W013",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W013",
			Description: "In an OCFL Object, extension sub-directories SHOULD be named according to a registered extension name.",
			URL:         "https://ocfl.io/1.1/spec/#W013",
		}
	default:
		return nil
	}
}

// W014: Storage hierarchies within the same OCFL Storage Root SHOULD use just one layout pattern.
func W014(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W014",
			Description: "Storage hierarchies within the same OCFL Storage Root SHOULD use just one layout pattern.",
			URL:         "https://ocfl.io/1.0/spec/#W014",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W014",
			Description: "Storage hierarchies within the same OCFL Storage Root SHOULD use just one layout pattern.",
			URL:         "https://ocfl.io/1.1/spec/#W014",
		}
	default:
		return nil
	}
}

// W015: Storage hierarchies within the same OCFL Storage Root SHOULD consistently use either a directory hierarchy of OCFL Objects or top-level OCFL Objects.
func W015(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.0"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W015",
			Description: "Storage hierarchies within the same OCFL Storage Root SHOULD consistently use either a directory hierarchy of OCFL Objects or top-level OCFL Objects.",
			URL:         "https://ocfl.io/1.0/spec/#W015",
		}
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W015",
			Description: "Storage hierarchies within the same OCFL Storage Root SHOULD consistently use either a directory hierarchy of OCFL Objects or top-level OCFL Objects.",
			URL:         "https://ocfl.io/1.1/spec/#W015",
		}
	default:
		return nil
	}
}

// W016: In the Storage Root, extension sub-directories SHOULD be named according to a registered extension name.
func W016(spec ocfl.Spec) *ocfl.ValidationCode {
	switch spec {
	case ocfl.Spec("1.1"):
		return &ocfl.ValidationCode{
			Spec:        spec,
			Code:        "W016",
			Description: "In the Storage Root, extension sub-directories SHOULD be named according to a registered extension name.",
			URL:         "https://ocfl.io/1.1/spec/#W016",
		}
	default:
		return nil
	}
}