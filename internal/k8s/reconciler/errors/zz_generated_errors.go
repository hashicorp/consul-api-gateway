package errors

// GENERATED from errors.yaml, DO NOT EDIT DIRECTLY

type CertificateResolutionErrorType int

const (
	CertificateResolutionErrorTypeNotFound CertificateResolutionErrorType = iota
	CertificateResolutionErrorTypeNotPermitted
	CertificateResolutionErrorTypeUnsupported
)

type CertificateResolutionError struct {
	inner     string
	errorType CertificateResolutionErrorType
}

func NewCertificateResolutionErrorNotFound(inner string) CertificateResolutionError {
	return CertificateResolutionError{inner, CertificateResolutionErrorTypeNotFound}
}
func NewCertificateResolutionErrorNotPermitted(inner string) CertificateResolutionError {
	return CertificateResolutionError{inner, CertificateResolutionErrorTypeNotPermitted}
}
func NewCertificateResolutionErrorUnsupported(inner string) CertificateResolutionError {
	return CertificateResolutionError{inner, CertificateResolutionErrorTypeUnsupported}
}

func (r CertificateResolutionError) Error() string {
	return r.inner
}

func (r CertificateResolutionError) Kind() CertificateResolutionErrorType {
	return r.errorType
}

type BindErrorType int

const (
	BindErrorTypeRouteKind BindErrorType = iota
	BindErrorTypeListenerNamespacePolicy
	BindErrorTypeHostnameMismatch
	BindErrorTypeRouteInvalid
)

type BindError struct {
	inner     string
	errorType BindErrorType
}

func NewBindErrorRouteKind(inner string) BindError {
	return BindError{inner, BindErrorTypeRouteKind}
}
func NewBindErrorListenerNamespacePolicy(inner string) BindError {
	return BindError{inner, BindErrorTypeListenerNamespacePolicy}
}
func NewBindErrorHostnameMismatch(inner string) BindError {
	return BindError{inner, BindErrorTypeHostnameMismatch}
}
func NewBindErrorRouteInvalid(inner string) BindError {
	return BindError{inner, BindErrorTypeRouteInvalid}
}

func (r BindError) Error() string {
	return r.inner
}

func (r BindError) Kind() BindErrorType {
	return r.errorType
}
