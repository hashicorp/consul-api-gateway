package testing

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"time"

	"github.com/google/uuid"
)

var (
	DefaultTestCA                *CertificateInfo
	DefaultTestServerCertificate *CertificateInfo
	DefaultTestClientCertificate *CertificateInfo
)

func init() {
	rootCA, err := GenerateSignedCertificate(GenerateCertificateOptions{
		IsCA: true,
	})
	if err != nil {
		panic(err)
	}
	serverCert, err := GenerateSignedCertificate(GenerateCertificateOptions{
		CA:          rootCA,
		ServiceName: "server",
	})
	if err != nil {
		panic(err)
	}
	clientCert, err := GenerateSignedCertificate(GenerateCertificateOptions{
		CA:          rootCA,
		ServiceName: "client",
	})
	if err != nil {
		panic(err)
	}
	DefaultTestCA = rootCA
	DefaultTestServerCertificate = serverCert
	DefaultTestClientCertificate = clientCert
}

type CertificateInfo struct {
	Cert            *x509.Certificate
	CertBytes       []byte
	PrivateKey      *rsa.PrivateKey
	PrivateKeyBytes []byte
	X509            tls.Certificate
	spiffe          *url.URL
}

func DefaultCertificates() (*CertificateInfo, *CertificateInfo, *CertificateInfo) {
	return DefaultTestCA, DefaultTestServerCertificate, DefaultTestClientCertificate
}

func getSVIDRootURI() *url.URL {
	var svid url.URL
	svid.Scheme = "spiffe"
	svid.Host = uuid.New().String() + ".consul"
	return &svid
}

func getSVIDServiceURI(root *url.URL, hostOverride, pathOverride, service string) *url.URL {
	var svid url.URL
	svid.Scheme = "spiffe"
	svid.Host = root.Host
	if hostOverride != "" {
		svid.Host = hostOverride
	}
	svid.Path = "/ns/default/dc/testing/svc/" + service
	if pathOverride != "" {
		svid.Path = pathOverride
	}
	return &svid
}

type GenerateCertificateOptions struct {
	CA                 *CertificateInfo
	IsCA               bool
	ServiceName        string
	SPIFFEHostOverride string
	SPIFFEPathOverride string
}

func GenerateSignedCertificate(options GenerateCertificateOptions) (*CertificateInfo, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}
	usage := x509.KeyUsageDigitalSignature
	if options.IsCA {
		usage = x509.KeyUsageCertSign
	}

	// 5. Validation
	// This section describes how an X.509 SVID is validated. The procedure uses standard X.509 validation, in addition to a small set of SPIFFE-specific validation steps.
	//
	// 5.1. Path Validation
	// The validation of trust in a given SVID is based on standard X.509 path validation, and MUST follow RFC 5280 path validation semantics.
	//
	// Certificate path validation requires the leaf SVID certificate and one or more SVID signing certificates. The set of signing certificates required for validation is known as the CA bundle. The mechanism through which an entity can retrieve the relevant CA bundle(s) is out of scope for this document, and is instead defined in the SPIFFE Workload API specification.
	//
	// 5.2. Leaf Validation
	// When authenticating a resource or caller, it is necessary to perform validation beyond what is covered by the X.509 standard. Namely, we must ensure that 1) the certificate is a leaf certificate, and 2) that the signing authority was authorized to issue it.
	//
	// When validating an X.509 SVID for authentication purposes, the validator MUST ensure that the CA field in the basic constraints extension is set to false, and that keyCertSign and cRLSign are not set in the key usage extension. The validator must also ensure that the scheme of the SPIFFE ID is set to spiffe://.
	//
	// As support for URI name constraints becomes more widespread, future versions of this document may update the requirements set forth in this section in order to better leverage name constraint validation.

	spiffe := getSVIDRootURI()
	if options.CA != nil {
		spiffe = getSVIDServiceURI(options.CA.spiffe, options.SPIFFEHostOverride, options.SPIFFEPathOverride, options.ServiceName)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		URIs:         []*url.URL{spiffe},
		Subject: pkix.Name{
			Organization:  []string{"Testing, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Fake Street"},
			PostalCode:    []string{"11111"},
		},
		IsCA:                  options.IsCA,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		SubjectKeyId:          []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              usage,
		BasicConstraintsValid: true,
	}
	caCert := cert
	if options.CA != nil {
		caCert = options.CA.Cert
	}
	caPrivateKey := privateKey
	if options.CA != nil {
		caPrivateKey = options.CA.PrivateKey
	}
	data, err := x509.CreateCertificate(rand.Reader, cert, caCert, &privateKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}

	var certificatePEM bytes.Buffer
	var privateKeyPEM bytes.Buffer
	pem.Encode(&certificatePEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: data,
	})
	pem.Encode(&privateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	certBytes := certificatePEM.Bytes()
	privateKeyBytes := privateKeyPEM.Bytes()
	x509Cert, err := tls.X509KeyPair(certBytes, privateKeyBytes)
	if err != nil {
		return nil, err
	}

	return &CertificateInfo{
		Cert:            cert,
		CertBytes:       certBytes,
		PrivateKey:      privateKey,
		PrivateKeyBytes: privateKeyBytes,
		X509:            x509Cert,
		spiffe:          spiffe,
	}, nil
}
