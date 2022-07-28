package vault

import (
	"errors"
	"fmt"
	"net/url"
)

var (
	ErrInvalidSecret = errors.New("invalid vault secret")
)

const (
	queryKeyAltNames  = "altNames"
	queryKeyIPSANs    = "ipSans"
	queryKeyOtherSANs = "otherSans"
	queryKeyTTL       = "ttl"
)

// Secret is a wrapper to a certificate secret stored in Vault.
//
// This Vault-specific implementation corresponds with the K8s-specific
// implementation, utils.K8sSecret.
type Secret struct {
	AltNames   string
	CommonName string
	IPSANs     string
	OtherSANs  string
	TTL        string
}

// NewSecret creates a descriptor for a certificate to be generated via Vault's PKI API.
// The arguments correspond with inputs to the cert + key generation endpoint.
// https://www.vaultproject.io/api-docs/secret/pki#generate-certificate-and-key
func NewSecret(commonName, altNames, ipSANs, otherSANs, ttl string) Secret {
	return Secret{
		AltNames:   altNames,
		CommonName: commonName,
		IPSANs:     ipSANs,
		OtherSANs:  otherSANs,
		TTL:        ttl,
	}
}

// ParseSecret accepts an opaque string reference and returns a Secret. The expected format
// is vault://<common_name>?ttl=<ttl>&ipSans=<sans>... where "common_name", "ttl", etc.
// correlate with values accepted by Vault's PKI API. Plural vars are generally comma-delimited
// lists as described in the docs.
//
// https://www.vaultproject.io/api-docs/secret/pki
//
// Some components such as the issuer and issue are configured globally today.
// In the future, we could include those as additional query parameters.
func ParseSecret(ref string) (Secret, error) {
	parsed, err := url.Parse(ref)
	if err != nil {
		return Secret{}, err
	}

	if parsed.Scheme != SecretScheme {
		return Secret{}, ErrInvalidSecret
	}

	commonName := parsed.Host
	altNames := parsed.Query().Get(queryKeyAltNames)
	ipSANs := parsed.Query().Get(queryKeyIPSANs)
	otherSANs := parsed.Query().Get(queryKeyOtherSANs)
	ttl := parsed.Query().Get(queryKeyTTL)

	return NewSecret(commonName, altNames, ipSANs, otherSANs, ttl), nil
}

// String serializes a Secret into an opaque string that can later
// be parsed and restored to an equivalent Secret.
func (s Secret) String() string {
	return (&url.URL{
		Scheme: SecretScheme,
		Path:   fmt.Sprintf("/%s", s.CommonName),
		RawQuery: url.Values{
			queryKeyAltNames:  []string{s.AltNames},
			queryKeyIPSANs:    []string{s.IPSANs},
			queryKeyOtherSANs: []string{s.OtherSANs},
			queryKeyTTL:       []string{s.TTL},
		}.Encode(),
	}).String()
}
