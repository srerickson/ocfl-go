package errors
// This is generated code. Do not modify. See gen folder.

// OCFLCodeErr represents an OCFL Validation Codes:
// see https://ocfl.io/validation/validation-codes.html
type OCFLCodeErr struct {
	Description string // description from spec
	Code        string // code from spec
	URI         string // reference URI from spec
}

{{range .}}
//Err{{index . 0}}: {{index . 1}}
var Err{{index . 0}} = OCFLCodeErr{
	Description: "{{index . 1}}",
	Code:        "{{index . 0}}",
	URI:         "{{index . 2}}",
}
{{end}}