package ocfl

import "fmt"

// Error Codes
const (
	ReadErr            int8 = iota + 1 // File System Error
	PathErr                            // could not determine absolute path to object or content
	NamasteErr                         // missing/invalid Object declaration (namaste file)
	VerFormatErr                       // inconsistent version name format
	InvJSONErr                         // error unmarshalling inventory json
	InvSidecarErr                      // missing inventory.json checksum sidecard
	InvChecksumErr                     // invalid inventory.json checksum
	InvIDErr                           // inventory has no id
	InvTypeErr                         // inventory has invalid/missing type
	InvDigestErr                       // inventory has invalid/missing digestAlgorthm
	InvNoManErr                        // no manifest
	InvNoVerErr                        // no versions
	ManDigestErr                       // manifest does not include expected digest
	ManPathErr                         // manifest does not include expected path
	ContentChecksumErr                 //content checksum does not match manifest`
	PathFormatErr                      // invalid path format (not relative or out of scope)
	CtxCanceledErr                     // context canceled
)

var errShortMessages = map[int8]string{
	ReadErr:            `file system error`,
	PathErr:            `could not determine absolute path to object or content`,
	NamasteErr:         `missing/invalid OCFL Object declaration`,
	VerFormatErr:       `inconsistent version name format`,
	InvJSONErr:         `error unmarshalling inventory json`,
	InvSidecarErr:      `missing inventory.json checksum sidecar`,
	InvChecksumErr:     `invalid inventory.json checksum`,
	InvIDErr:           `inventory has no id`,
	InvTypeErr:         `inventory has invalid/missing type`,
	InvDigestErr:       `inventory has invalid/missing digestAlgorthm`,
	InvNoManErr:        `inventory has no manifest`,
	InvNoVerErr:        `inventory has no versions`,
	ManDigestErr:       `manifest does not include expected digest`,
	ManPathErr:         `manifest does not include expected path`,
	ContentChecksumErr: `content checksum does not match manifest`,
	PathFormatErr:      `invalid path in inventory`,
	CtxCanceledErr:     `context was canceled`,
}

// Error is interface is interface for error with codes
type Error interface {
	error
	Code() int8
}

// ObjectError implements Error interface
type ObjectError struct {
	error
	code int8
}

// NewErr creates a new error
func NewErr(code int8, err error) Error {
	return ObjectError{code: code, error: err}
}

// NewErrf creates a new error using format syntax
func NewErrf(code int8, message string, v ...interface{}) Error {
	return NewErr(code, fmt.Errorf(message, v...))
}

func (err ObjectError) Error() string {
	if err.error != nil {
		return err.error.Error()
	}
	return errShortMessages[err.code]
}

// Code returns error code
func (err ObjectError) Code() int8 {
	return err.code
}
