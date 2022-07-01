package validation

// ErrorCode is an error that also references an OCFL spec.
type ErrorCode interface {
	error
	// Unwrap() error
	OCFLRef() *Ref
}

func NewErrorCode(err error, ref *Ref) ErrorCode {
	return &vErr{error: err, ref: ref}
}

// VErr is an error returned from validation check
type vErr struct {
	error
	ref *Ref // code from spec
}

func (verr *vErr) OCFLRef() *Ref {
	return verr.ref
}

func (verr *vErr) Unwrap() error {
	return verr.error
}

func (verr *vErr) Code() string {
	if verr.ref == nil {
		return ""
	}
	return verr.ref.Code
}

func (verr *vErr) Description() string {
	if verr.ref == nil {
		return ""
	}
	return verr.ref.Description
}

func (verr *vErr) URL() string {
	if verr.ref == nil {
		return ""
	}
	return verr.ref.URL
}
