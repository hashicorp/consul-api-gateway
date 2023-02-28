// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package errors

// GENERATED from errors.yaml, DO NOT EDIT DIRECTLY

type CertificateResolutionErrorType string

const (
	CertificateResolutionErrorTypeNotFound     CertificateResolutionErrorType = "NotFoundError"
	CertificateResolutionErrorTypeNotPermitted CertificateResolutionErrorType = "NotPermittedError"
	CertificateResolutionErrorTypeUnsupported  CertificateResolutionErrorType = "UnsupportedError"
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

type BindErrorType string

const (
	BindErrorTypeRouteKind               BindErrorType = "RouteKindError"
	BindErrorTypeListenerNamespacePolicy BindErrorType = "ListenerNamespacePolicyError"
	BindErrorTypeHostnameMismatch        BindErrorType = "HostnameMismatchError"
	BindErrorTypeRouteInvalid            BindErrorType = "RouteInvalidError"
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
