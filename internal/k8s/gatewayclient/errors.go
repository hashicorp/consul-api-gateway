package gatewayclient

// K8sError is an error type that should wrap any Kubernetes API
// errors that the gatewayclient returns -- they're caught in
// the requeueing middleware to be retried immediately rather
// than with a delayed requeue.
type K8sError struct {
	inner error
}

func NewK8sError(err error) K8sError {
	return K8sError{inner: err}
}

func (s K8sError) Error() string {
	return s.inner.Error()
}
