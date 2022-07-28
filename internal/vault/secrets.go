package vault

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrInvalidSecret = errors.New("invalid vault secret")
)

// Secret is a wrapper to a certificate secret stored in Vault.
//
// This Vault-specific implementation corresponds with the K8s-specific
// implementation, utils.K8sSecret.
type Secret struct {
	CommonName string
	TTL        string
}

func NewSecret(commonName, ttl string) Secret {
	return Secret{
		CommonName: commonName,
		TTL:        ttl,
	}
}

// ParseSecret accepts an opaque string reference and returns a Secret.
// The expected format is vault:///<common_name>?ttl=<ttl> where "common_name"
// and "ttl" correlate with values accepted by Vault's PKI API.
func ParseSecret(ref string) (Secret, error) {
	parsed, err := url.Parse(ref)
	if err != nil {
		return Secret{}, err
	}

	if parsed.Scheme != SecretScheme {
		return Secret{}, ErrInvalidSecret
	}

	if !strings.HasPrefix(parsed.Path, "/") {
		return Secret{}, ErrInvalidSecret
	}

	ref = strings.TrimPrefix(parsed.Path, "/")
	tokens := strings.SplitN(ref, "/", 2)
	if len(tokens) != 2 {
		return Secret{}, ErrInvalidSecret
	}

	return NewSecret(tokens[0], tokens[1]), nil
}

// String serializes a Secret into an opaque string that can later
// be parsed and restored to an equivalent Secret.
func (s Secret) String() string {
	return (&url.URL{
		Scheme: SecretScheme,
		Path:   fmt.Sprintf("/%s", s.CommonName),
		RawQuery: url.Values{
			"ttl": []string{s.TTL},
		}.Encode(),
	}).String()
}
