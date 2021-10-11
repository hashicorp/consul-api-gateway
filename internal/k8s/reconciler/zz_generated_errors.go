package reconciler

// GENERATED from errors.yaml, DO NOT EDIT DIRECTLY

type CertificateResolutionErrorType int

const (
	CertificateResolutionErrorTypeNotFound CertificateResolutionErrorType = iota
	CertificateResolutionErrorTypeUnsupported
)

type CertificateResolutionError struct {
	inner     string
	errorType CertificateResolutionErrorType
}

func NewCertificateResolutionErrorNotFound(inner string) CertificateResolutionError {
	return CertificateResolutionError{inner, CertificateResolutionErrorTypeNotFound}
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

func (r BindError) Error() string {
	return r.inner
}

func (r BindError) Kind() BindErrorType {
	return r.errorType
}
