package utils

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrNotK8sSecret     = errors.New("not a kubernetes secret")
	ErrInvalidK8sSecret = errors.New("invalid kubernetes secret")
)

const (
	K8sSecretScheme = "k8s"
)

// KK8sSecret is a wrapper to a Kubernetes certificate secret
type K8sSecret struct {
	Namespace string
	Name      string
}

// NewK8sSecret returns an K8sSecret object
func NewK8sSecret(namespace, name string) K8sSecret {
	return K8sSecret{
		Namespace: namespace,
		Name:      name,
	}
}

// ParseK8sSecret parses an encoded kubernetes secret.
// If the encoded secret is not a Kubernetes secret or
// it can't be parsed, it returns an error.
func ParseK8sSecret(encoded string) (K8sSecret, error) {
	parsed, err := url.Parse(encoded)
	if err != nil {
		return K8sSecret{}, err
	}
	if parsed.Scheme != K8sSecretScheme {
		return K8sSecret{}, ErrNotK8sSecret
	}
	if !strings.HasPrefix(parsed.Path, "/") {
		return K8sSecret{}, ErrInvalidK8sSecret
	}
	path := strings.TrimPrefix(parsed.Path, "/")
	tokens := strings.SplitN(path, "/", 2)
	if len(tokens) != 2 {
		return K8sSecret{}, ErrInvalidK8sSecret
	}
	return NewK8sSecret(tokens[0], tokens[1]), nil
}

// String returns a kubernetes secret encoded as a string
func (k K8sSecret) String() string {
	path := fmt.Sprintf("/%s/%s", k.Namespace, k.Name)
	return (&url.URL{
		Scheme: K8sSecretScheme,
		Path:   path,
	}).String()
}
