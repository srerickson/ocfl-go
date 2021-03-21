package ocfl

// This is generated code. Do not modify. See errors_gen folder.

{{range .}}
var Err{{index . 0}} = ObjectValidationErr{
	Description: "{{index . 1}}",
	Code: "{{index . 0}}",
	URI: "{{index . 2}}",
}

{{end}}