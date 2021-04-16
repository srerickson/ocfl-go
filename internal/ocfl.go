package internal

const (
	ocflVersion           = "1.0"
	objectDeclaration     = `ocfl_object_` + ocflVersion
	objectDeclarationFile = `0=ocfl_object_` + ocflVersion
	inventoryFile         = `inventory.json`

	// defaults
	inventoryType   = `https://ocfl.io/1.0/spec/#inventory`
	contentDir      = `content`
	digestAlgorithm = "sha512"
)
