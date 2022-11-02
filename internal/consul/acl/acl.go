package acl

import (
	"errors"
	"strings"
)

// These error constants define the standard ACL error types. The values
// must not be changed since the error values are sent via RPC calls
// from older clients and may not have the correct type.
const (
	errNotFound         = "ACL not found"
	errPermissionDenied = "Permission denied"
)

var (
	// ErrNotFound indicates there is no matching ACL.
	ErrNotFound = errors.New(errNotFound)
)

// IsErrNotFound checks if the given error message is comparable to
// ErrNotFound.
func IsErrNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), errNotFound)
}

// IsErrPermissionDenied checks if the given error message is comparable
// to ErrPermissionDenied.
func IsErrPermissionDenied(err error) bool {
	return err != nil && strings.Contains(err.Error(), errPermissionDenied)
}
